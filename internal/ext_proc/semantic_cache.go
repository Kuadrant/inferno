package ext_proc

import (
	"encoding/json"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"sync"
	"time"

	extProcPb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typeV3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// CacheEntry holds prompt, its embedding, and the cached response
type CacheEntry struct {
	Prompt     string
	Embedding  []float64
	Response   []byte
	CreateTime time.Time
}

type SemanticCache struct {
	semanticCache       []*CacheEntry
	embeddingCache      sync.Map
	embeddingServerURL  string
	embeddingModelHost  string
	similarityThreshold float64
	cacheMutex          sync.Mutex
}

func NewSemanticCache() *SemanticCache {
	embeddingServerURL := os.Getenv("EMBEDDING_MODEL_SERVER")
	embeddingModelHost := os.Getenv("EMBEDDING_MODEL_HOST")
	log.Printf("[SemanticCache] EMBEDDING_MODEL_SERVER=%s", embeddingServerURL)
	log.Printf("[SemanticCache] EMBEDDING_MODEL_HOST=%s", embeddingModelHost)

	similarityThreshold := 0.75
	if ts := os.Getenv("SIMILARITY_THRESHOLD"); ts != "" {
		if v, err := strconv.ParseFloat(ts, 64); err == nil {
			similarityThreshold = v
			log.Printf("[SemanticCache] similarityThreshold=%.3f", similarityThreshold)
		}
	}

	return &SemanticCache{
		semanticCache:       []*CacheEntry{},
		embeddingServerURL:  embeddingServerURL,
		embeddingModelHost:  embeddingModelHost,
		similarityThreshold: similarityThreshold,
	}
}

func (sc *SemanticCache) cosineSimilarity(a, b []float64) float64 {
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

func (sc *SemanticCache) findMostSimilarPrompt(vec []float64) (*CacheEntry, float64) {
	sc.cacheMutex.Lock()
	defer sc.cacheMutex.Unlock()
	var best *CacheEntry
	var bestSim float64
	for _, e := range sc.semanticCache {
		if s := sc.cosineSimilarity(vec, e.Embedding); s > bestSim {
			bestSim, best = s, e
		}
	}
	return best, bestSim
}

func (sc *SemanticCache) Process(srv extProcPb.ExternalProcessor_ProcessServer) error {
	log.Println("[SemanticCache] Starting processing loop")
	var lastPrompt string

	for {
		req, err := srv.Recv()
		if err == io.EOF {
			log.Println("[SemanticCache] EOF, exiting")
			return nil
		} else if err != nil {
			log.Printf("[SemanticCache] Recv error: %v", err)
			return status.Errorf(codes.Unknown, "recv error: %v", err)
		}

		var resp *extProcPb.ProcessingResponse
		log.Printf("[SemanticCache] Handling %T", req.Request)

		switch r := req.Request.(type) {

		case *extProcPb.ProcessingRequest_RequestHeaders:
			resp = &extProcPb.ProcessingResponse{Response: &extProcPb.ProcessingResponse_RequestHeaders{RequestHeaders: &extProcPb.HeadersResponse{}}}

		case *extProcPb.ProcessingRequest_RequestBody:
			rb := r.RequestBody
			log.Printf("[SemanticCache] RequestBody, end_of_stream=%v", rb.EndOfStream)
			if !rb.EndOfStream {
				srv.Send(&extProcPb.ProcessingResponse{Response: &extProcPb.ProcessingResponse_RequestBody{RequestBody: &extProcPb.BodyResponse{}}})
				continue
			}

			// parse JSON once complete
			var pl map[string]interface{}
			if err := json.Unmarshal(rb.Body, &pl); err != nil {
				log.Printf("[SemanticCache] JSON parse failed: %v", err)
				resp = &extProcPb.ProcessingResponse{Response: &extProcPb.ProcessingResponse_RequestBody{RequestBody: &extProcPb.BodyResponse{}}}
				break
			}

			// extract prompt
			if raw, ok := pl["prompt"]; ok {
				if prompt, ok2 := raw.(string); ok2 {
					log.Printf("[SemanticCache] Prompt: %s", prompt)
					lastPrompt = prompt

					// lookup embedding
					var emb []float64
					if v, ok3 := sc.embeddingCache.Load(prompt); ok3 {
						emb = v.([]float64)
						log.Println("[SemanticCache] Exact match cache hit for embedding")
					} else if sc.embeddingServerURL != "" {
						emb = fetchEmbedding(sc.embeddingServerURL, sc.embeddingModelHost, prompt)
						if emb != nil {
							sc.embeddingCache.Store(prompt, emb)
							log.Printf("[SemanticCache] Stored new embedding len=%d", len(emb))
						}
					}

					// similarity logging
					if len(emb) > 0 {
						log.Printf("[SemanticCache] Semantic lookup on %d entries", len(sc.semanticCache))
						e, sim := sc.findMostSimilarPrompt(emb)
						if e != nil {
							log.Printf("[SemanticCache] Best candidate: %s with similarity=%.3f (threshold=%.3f)", e.Prompt, sim, sc.similarityThreshold)
							if sim >= sc.similarityThreshold && e.Response != nil {
								log.Printf("[SemanticCache] similarity %.3f >= threshold %.3f; cache HIT", sim, sc.similarityThreshold)

								// extract token metrics headers from cached response
								headers := ExtractTokenMetricsHeaders(e.Response)

								if headers != nil {
									log.Printf("[SemanticCache] Found token metrics in cached response")
									// return cached response with token metrics headers
									srv.Send(&extProcPb.ProcessingResponse{
										Response: &extProcPb.ProcessingResponse_ImmediateResponse{
											ImmediateResponse: &extProcPb.ImmediateResponse{
												Status: &typeV3.HttpStatus{Code: 200},
												Body:   e.Response,
												Headers: &extProcPb.HeaderMutation{
													SetHeaders: headers,
												},
											},
										},
									})
								} else {
									// no metrics found, return cached response as-is
									srv.Send(&extProcPb.ProcessingResponse{
										Response: &extProcPb.ProcessingResponse_ImmediateResponse{
											ImmediateResponse: &extProcPb.ImmediateResponse{
												Status: &typeV3.HttpStatus{Code: 200},
												Body:   e.Response,
											},
										},
									})
								}
								continue
							} else {
								log.Printf("[SemanticCache] similarity %.3f < threshold %.3f; no cache hit", sim, sc.similarityThreshold)
							}
						} else {
							log.Println("[SemanticCache] semanticCache empty; no candidate to compare")
						}
					}
				}
			}

			// pass through request body
			resp = &extProcPb.ProcessingResponse{Response: &extProcPb.ProcessingResponse_RequestBody{RequestBody: &extProcPb.BodyResponse{}}}

		case *extProcPb.ProcessingRequest_ResponseHeaders:
			resp = &extProcPb.ProcessingResponse{Response: &extProcPb.ProcessingResponse_ResponseHeaders{ResponseHeaders: &extProcPb.HeadersResponse{}}}

		case *extProcPb.ProcessingRequest_ResponseBody:
			rb := r.ResponseBody
			log.Printf("[SemanticCache] ResponseBody, end_of_stream=%v", rb.EndOfStream)
			if rb.EndOfStream && lastPrompt != "" {
				sc.cacheMutex.Lock()
				if embI, ok := sc.embeddingCache.Load(lastPrompt); ok {
					emb := embI.([]float64)
					sc.semanticCache = append(sc.semanticCache, &CacheEntry{Prompt: lastPrompt, Embedding: emb, Response: rb.Body, CreateTime: time.Now()})
					log.Printf("[SemanticCache] Added semanticCache entry for %s", lastPrompt)
				}
				sc.cacheMutex.Unlock()
			}
			resp = &extProcPb.ProcessingResponse{Response: &extProcPb.ProcessingResponse_ResponseBody{ResponseBody: &extProcPb.BodyResponse{}}}

		default:
			resp = &extProcPb.ProcessingResponse{}
		}

		// send response
		srv.Send(resp)
	}
}
