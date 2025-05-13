package ext_proc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	filterPb "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	extProcPb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typeV3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Processor struct {
	semanticCache *SemanticCache
	promptGuard   *PromptGuard
	tokenMetrics  *TokenUsageMetrics
	prompts       sync.Map
}

func NewProcessor() *Processor {
	return &Processor{
		semanticCache: NewSemanticCache(),
		promptGuard:   NewPromptGuard(nil),
		tokenMetrics:  NewTokenUsageMetrics(),
		prompts:       sync.Map{},
	}
}

func (p *Processor) Process(srv extProcPb.ExternalProcessor_ProcessServer) error {
	log.Println("[Processor] Starting processing loop")

	for {
		req, err := srv.Recv()
		if err == io.EOF {
			log.Println("[Processor] Received EOF, terminating processing loop")
			return nil
		}
		if err != nil {
			if status.Code(err) == codes.Canceled {
				log.Println("[Processor] Stream cancelled, finishing up")
				return nil
			}
			log.Printf("[Processor] Error receiving request: %v", err)
			return err
		}

		log.Printf("[Processor] Received request type: %T", req.Request)

		var resp *extProcPb.ProcessingResponse

		switch r := req.Request.(type) {
		case *extProcPb.ProcessingRequest_RequestHeaders:
			log.Println("[Processor] Processing RequestHeaders")
			resp = &extProcPb.ProcessingResponse{
				Response: &extProcPb.ProcessingResponse_RequestHeaders{
					RequestHeaders: &extProcPb.HeadersResponse{},
				},
			}

		case *extProcPb.ProcessingRequest_RequestBody:
			log.Println("[Processor] Processing RequestBody")
			if !r.RequestBody.EndOfStream {
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_RequestBody{
						RequestBody: &extProcPb.BodyResponse{},
					},
				}
				break
			}

			// extract the prompt from the request body
			bodyMap := make(map[string]interface{})
			if err := json.Unmarshal(r.RequestBody.Body, &bodyMap); err != nil {
				log.Printf("[Processor] Failed to parse request body: %v", err)
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_RequestBody{
						RequestBody: &extProcPb.BodyResponse{},
					},
				}
				break
			}

			// extract the prompt
			prompt, err := extractPrompt(bodyMap)
			if err != nil {
				log.Printf("[Processor] %v", err)
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_RequestBody{
						RequestBody: &extProcPb.BodyResponse{},
					},
				}
				break
			}

			// store the prompt for later use with responses
			requestID := fmt.Sprintf("%p", srv)
			p.prompts.Store(requestID, prompt)

			// check if the prompt is risky
			if os.Getenv("DISABLE_PROMPT_RISK_CHECK") != "yes" {
				requestCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if p.promptGuard.CheckRisk(requestCtx, prompt) {
					log.Println("[Processor] Risky prompt detected, returning 403")
					resp = createForbiddenResponse("Prompt blocked by content policy")
					break
				}
			}

			// check if we have a cached response
			var emb []float64
			if v, ok := p.semanticCache.embeddingCache.Load(prompt); ok {
				emb = v.([]float64)
				log.Println("[Processor] Exact match cache hit for embedding")
			} else if p.semanticCache.embeddingServerURL != "" {
				// fetch embedding from server
				emb = fetchEmbedding(p.semanticCache.embeddingServerURL,
					p.semanticCache.embeddingModelHost,
					prompt)
				if len(emb) > 0 {
					p.semanticCache.embeddingCache.Store(prompt, emb)
				}
			}

			// if we have an embedding, try to find similar prompts
			if len(emb) > 0 {
				e, sim := p.semanticCache.findMostSimilarPrompt(emb)
				if e != nil && sim >= p.semanticCache.similarityThreshold && e.Response != nil {
					log.Printf("[Processor] Semantic cache hit with similarity %.3f", sim)

					// return cached response
					resp = &extProcPb.ProcessingResponse{
						Response: &extProcPb.ProcessingResponse_ImmediateResponse{
							ImmediateResponse: &extProcPb.ImmediateResponse{
								Status: &typeV3.HttpStatus{Code: 200},
								Body:   e.Response,
							},
						},
					}
					break
				}
			}

			// if we get here, just pass through the request
			resp = &extProcPb.ProcessingResponse{
				Response: &extProcPb.ProcessingResponse_RequestBody{
					RequestBody: &extProcPb.BodyResponse{},
				},
			}

		case *extProcPb.ProcessingRequest_ResponseHeaders:
			log.Println("[Processor] Processing ResponseHeaders")
			// both prompt guard and token metrics need to process response, so we want to buffer the body
			resp = &extProcPb.ProcessingResponse{
				Response: &extProcPb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &extProcPb.HeadersResponse{},
				},
				ModeOverride: &filterPb.ProcessingMode{
					ResponseHeaderMode: filterPb.ProcessingMode_SKIP,
					ResponseBodyMode:   filterPb.ProcessingMode_BUFFERED,
				},
			}

		case *extProcPb.ProcessingRequest_ResponseBody:
			log.Println("[Processor] Processing ResponseBody")

			// If not end of stream, just continue buffering
			if !r.ResponseBody.EndOfStream {
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_ResponseBody{
						ResponseBody: &extProcPb.BodyResponse{},
					},
				}
				break
			}

			// check for harmful responses if configured
			if os.Getenv("DISABLE_RESPONSE_RISK_CHECK") != "yes" {
				// Parse the response to extract generated text
				respData := make(map[string]interface{})
				if err := json.Unmarshal(r.ResponseBody.Body, &respData); err == nil {
					var generated string
					choices, ok := respData["choices"].([]interface{})
					if ok && len(choices) > 0 {
						first, _ := choices[0].(map[string]interface{})
						generated, _ = first["text"].(string)

						// Use prompt guard's CheckRisk function if we have generated text
						if generated != "" {
							responseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
							defer cancel()
							if p.promptGuard.CheckRisk(responseCtx, generated) {
								log.Println("[Processor] Risky LLM output detected, blocking response")
								resp = createForbiddenResponse("LLM output blocked by safety filter")
								break
							}
						}
					}
				}
			}

			// store in semantic cache
			requestID := fmt.Sprintf("%p", srv)
			if promptI, ok := p.prompts.Load(requestID); ok {
				prompt := promptI.(string)
				log.Printf("[Processor] Found prompt '%s' for caching response", prompt)

				// get the embedding for this prompt
				if embI, ok := p.semanticCache.embeddingCache.Load(prompt); ok {
					emb := embI.([]float64)
					p.semanticCache.cacheMutex.Lock()
					p.semanticCache.semanticCache = append(p.semanticCache.semanticCache,
						&CacheEntry{
							Prompt:     prompt,
							Embedding:  emb,
							Response:   r.ResponseBody.Body,
							CreateTime: time.Now(),
						})
					p.semanticCache.cacheMutex.Unlock()
					log.Printf("[Processor] Added semanticCache entry for %s", prompt)
				}

				p.prompts.Delete(requestID)
			}

			// Process token metrics using the dedicated TokenUsageMetrics component
			var processResp *extProcPb.ProcessingResponse
			var metricsFound bool

			// Use the TokenUsageMetrics component to process the response body
			processResp, metricsFound = p.tokenMetrics.ProcessResponseBody(r.ResponseBody.Body)

			if metricsFound {
				// Use the response with token usage headers
				resp = processResp
			} else {
				// If metrics weren't found, just pass through
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_ResponseBody{
						ResponseBody: &extProcPb.BodyResponse{},
					},
				}
			}

		default:
			log.Printf("[Processor] Unrecognized request type: %T", req.Request)
			resp = &extProcPb.ProcessingResponse{}
		}

		if err := srv.Send(resp); err != nil {
			log.Printf("[Processor] Error sending response: %v", err)
			return err
		}
	}
}
