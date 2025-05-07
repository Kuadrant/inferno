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
	log.Println("[Processor] Cache miss, fetching embedding from", embeddingServerURL)
	reqMap := map[string]interface{}{"instances": []string{prompt}}
	data, _ := json.Marshal(reqMap)
	client := &http.Client{Timeout: 10 * time.Second}
	httpReq, err := http.NewRequest("POST", embeddingServerURL, bytes.NewReader(data))
	if err != nil {
		log.Printf("[Processor] HTTP request err: %v", err)
		return nil
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if embeddingModelHost != "" {
		httpReq.Host = embeddingModelHost
		log.Printf("[Processor] Set Host header: %s", embeddingModelHost)
	}

	httpResp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[Processor] Fetch embedding err: %v", err)
		return nil
	}

	log.Printf("[Processor] Embedding responded: %s", httpResp.Status)
	b, _ := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()

	var o struct{ Predictions [][]float64 }
	if json.Unmarshal(b, &o) == nil && len(o.Predictions) > 0 {
		return o.Predictions[0]
	}

	return nil
}
