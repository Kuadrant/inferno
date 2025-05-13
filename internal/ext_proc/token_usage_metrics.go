package ext_proc

import (
	"encoding/json"
	"io"
	"log"
	"strconv"

	configPb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	filterPb "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	extProcPb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type TokenUsageMetrics struct {
}

func NewTokenUsageMetrics() *TokenUsageMetrics {
	return &TokenUsageMetrics{}
}

// extracts token usage metrics from the response body and returns appropriate headers
// returns a processing response with the token usage headers and a boolean indicating if metrics were found
func (tm *TokenUsageMetrics) ProcessResponseBody(body []byte) (*extProcPb.ProcessingResponse, bool) {
	log.Println("[TokenMetrics] Processing response body for token metrics")

	// parse OpenAI-style usage metrics
	var openAIResp struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			TotalTokens      int `json:"total_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	err := json.Unmarshal(body, &openAIResp)
	if err != nil {
		log.Printf("[TokenMetrics] Failed to unmarshal JSON for token metrics: %v", err)
		return &extProcPb.ProcessingResponse{
			Response: &extProcPb.ProcessingResponse_ResponseBody{
				ResponseBody: &extProcPb.BodyResponse{},
			},
		}, false
	}

	log.Printf("[TokenMetrics] Successfully parsed usage metrics: %+v", openAIResp.Usage)

	// Create headers with token usage information
	headers := []*configPb.HeaderValueOption{
		{
			Header: &configPb.HeaderValue{
				Key:      "x-kuadrant-openai-prompt-tokens",
				RawValue: []byte(strconv.Itoa(openAIResp.Usage.PromptTokens)),
			},
		},
		{
			Header: &configPb.HeaderValue{
				Key:      "x-kuadrant-openai-total-tokens",
				RawValue: []byte(strconv.Itoa(openAIResp.Usage.TotalTokens)),
			},
		},
		{
			Header: &configPb.HeaderValue{
				Key:      "x-kuadrant-openai-completion-tokens",
				RawValue: []byte(strconv.Itoa(openAIResp.Usage.CompletionTokens)),
			},
		},
	}

	resp := &extProcPb.ProcessingResponse{
		Response: &extProcPb.ProcessingResponse_ResponseBody{
			ResponseBody: &extProcPb.BodyResponse{
				Response: &extProcPb.CommonResponse{
					HeaderMutation: &extProcPb.HeaderMutation{
						SetHeaders: headers,
					},
				},
			},
		},
	}

	log.Printf("[TokenMetrics] ResponseBody processed and decorated with headers: %+v", headers)
	return resp, true
}

func (tm *TokenUsageMetrics) Process(srv extProcPb.ExternalProcessor_ProcessServer) error {
	log.Println("[TokenMetrics] Starting processing loop")
	for {
		req, err := srv.Recv()
		if err == io.EOF {
			log.Println("[TokenMetrics] Received EOF, terminating processing loop")
			return nil
		}
		if err != nil {
			log.Printf("[TokenMetrics] Error receiving request: %v", err)
			return status.Errorf(codes.Unknown, "cannot receive stream request: %v", err)
		}

		log.Printf("[TokenMetrics] Received request: %+v", req)

		var resp *extProcPb.ProcessingResponse

		switch r := req.Request.(type) {
		case *extProcPb.ProcessingRequest_RequestHeaders:
			log.Println("[TokenMetrics] Processing RequestHeaders")
			// pass through headers untouched
			resp = &extProcPb.ProcessingResponse{
				Response: &extProcPb.ProcessingResponse_RequestHeaders{
					RequestHeaders: &extProcPb.HeadersResponse{},
				},
			}
			log.Println("[TokenMetrics] RequestHeaders processed, passing through response unchanged")

		case *extProcPb.ProcessingRequest_RequestBody:
			log.Println("[TokenMetrics] Processing RequestBody")
			// pass body untouched
			resp = &extProcPb.ProcessingResponse{
				Response: &extProcPb.ProcessingResponse_RequestBody{
					RequestBody: &extProcPb.BodyResponse{},
				},
			}
			log.Println("[TokenMetrics] RequestBody processed, passing through response unchanged")

		case *extProcPb.ProcessingRequest_ResponseHeaders:
			log.Println("[TokenMetrics] Processing ResponseHeaders, instructing Envoy to buffer response body")
			// buffer the response body
			resp = &extProcPb.ProcessingResponse{
				Response: &extProcPb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &extProcPb.HeadersResponse{},
				},
				ModeOverride: &filterPb.ProcessingMode{
					ResponseHeaderMode: filterPb.ProcessingMode_SKIP,
					ResponseBodyMode:   filterPb.ProcessingMode_BUFFERED,
				},
			}
			log.Println("[TokenMetrics] ResponseHeaders processed, buffering response body")

		case *extProcPb.ProcessingRequest_ResponseBody:
			log.Println("[TokenMetrics] Processing ResponseBody")
			rb := r.ResponseBody
			log.Printf("[TokenMetrics] ResponseBody received, EndOfStream: %v", rb.EndOfStream)
			if !rb.EndOfStream {
				log.Println("[TokenMetrics] ResponseBody not complete, continuing to buffer")
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_ResponseBody{
						ResponseBody: &extProcPb.BodyResponse{},
					},
				}
				break
			}

			log.Println("[TokenMetrics] Received complete ResponseBody, using ProcessResponseBody method")

			// Use the shared method to process the response body
			var metricsFound bool
			resp, metricsFound = tm.ProcessResponseBody(rb.Body)

			if !metricsFound {
				log.Println("[TokenMetrics] No metrics found in response, passing through")
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_ResponseBody{
						ResponseBody: &extProcPb.BodyResponse{},
					},
				}
			}
			log.Printf("[TokenMetrics] ResponseBody processed: metrics found = %v", metricsFound)

		default:
			log.Printf("[TokenMetrics] Received unrecognized request type: %+v", r)
			resp = &extProcPb.ProcessingResponse{}
		}

		if err := srv.Send(resp); err != nil {
			log.Printf("[TokenMetrics] Error sending response: %v", err)
			return err
		} else {
			log.Printf("[TokenMetrics] Sent response: %+v", resp)
		}
	}
}
