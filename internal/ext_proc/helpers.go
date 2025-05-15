package ext_proc

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	configPb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extProcPb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typeV3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
)

func createForbiddenResponse(message string) *extProcPb.ProcessingResponse {
	return &extProcPb.ProcessingResponse{
		Response: &extProcPb.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extProcPb.ImmediateResponse{
				Status: &typeV3.HttpStatus{
					Code: typeV3.StatusCode_Forbidden,
				},
				Body: []byte(`{"error":"` + message + `"}`),
				Headers: &extProcPb.HeaderMutation{
					SetHeaders: []*configPb.HeaderValueOption{
						{
							Header: &configPb.HeaderValue{
								Key:   "Content-Type",
								Value: "application/json",
							},
						},
					},
				},
			},
		},
	}
}

func fetchEmbedding(embeddingServerURL, embeddingModelHost, prompt string) []float64 {
	log.Printf("[SemanticCache] Cache miss, fetching embedding from %s", embeddingServerURL)
	reqMap := map[string]interface{}{"instances": []string{prompt}}
	data, _ := json.Marshal(reqMap)

	client := &http.Client{Timeout: 10 * time.Second}
	httpReq, err := http.NewRequest("POST", embeddingServerURL, bytes.NewReader(data))
	if err != nil {
		log.Printf("[SemanticCache] HTTP request creation err: %v", err)
		return nil
	}

	if embeddingModelHost != "" {
		httpReq.Host = embeddingModelHost
		log.Printf("[SemanticCache] Set Host header: %s", embeddingModelHost)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// cURL req for reproducing
	log.Printf("[SemanticCache] curl -X %s '%s' -H 'Host: %s' -H 'Content-Type: application/json' --data '%s'",
		httpReq.Method, httpReq.URL.String(), httpReq.Host, string(data))

	httpResp, err := client.Do(httpReq)

	if err != nil {
		log.Printf("[SemanticCache][ERROR] Fetch embedding err: %v", err)
		return nil
	}
	defer httpResp.Body.Close()

	log.Printf("[SemanticCache] Embedding responded: %s", httpResp.Status)

	if httpResp.StatusCode != http.StatusOK {
		log.Printf("[SemanticCache] Unexpected status code: %d", httpResp.StatusCode)
		if allow := httpResp.Header.Get("Allow"); allow != "" {
			log.Printf("[SemanticCache] Allowed methods: %s", allow)
		}
	}

	b, _ := io.ReadAll(httpResp.Body)
	log.Printf("[SemanticCache] Response body (%d bytes): %s", len(b), string(b))

	var o struct{ Predictions [][]float64 }
	if err := json.Unmarshal(b, &o); err != nil {
		log.Printf("[SemanticCache][ERROR] JSON unmarshal err: %v", err)
		return nil
	}
	if len(o.Predictions) == 0 {
		log.Println("[SemanticCache] No predictions found in response")
		return nil
	}

	emb := o.Predictions[0]
	return emb
}
