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
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type TokenUsageMetrics struct {
}

func NewTokenUsageMetrics() *TokenUsageMetrics {
	return &TokenUsageMetrics{}
}

// ExtractTokenMetricsHeaders processes a response body for token usage metrics
// and returns the headers if found, or nil if no metrics were found
func ExtractTokenMetricsHeaders(responseBody []byte) []*configPb.HeaderValueOption {
	tm := &TokenUsageMetrics{}
	metricsResp, metricsFound := tm.ProcessResponseBody(responseBody)
	
	if !metricsFound {
		return nil
	}
	
	// Extract headers from token metrics response
	if respBody, ok := metricsResp.Response.(*extProcPb.ProcessingResponse_ResponseBody); ok {
		if respBody.ResponseBody.Response != nil && respBody.ResponseBody.Response.HeaderMutation != nil {
			return respBody.ResponseBody.Response.HeaderMutation.SetHeaders
		}
	}
	
	return nil
}

// extracts token usage metrics from the response body and returns appropriate headers
// returns a processing response with the token usage headers and a boolean indicating if metrics were found
func (tm *TokenUsageMetrics) ProcessResponseBody(body []byte) (*extProcPb.ProcessingResponse, bool) {
	log.Println("[TokenMetrics] Processing response body")

	if !json.Valid(body) {
		log.Printf("[TokenMetrics] Response body is not valid JSON")
		return &extProcPb.ProcessingResponse{
			Response: &extProcPb.ProcessingResponse_ResponseBody{
				ResponseBody: &extProcPb.BodyResponse{},
			},
		}, false
	}

	// try to unmarshal into a map to check for usage field existence
	var responseMap map[string]interface{}
	if err := json.Unmarshal(body, &responseMap); err != nil {
		log.Printf("[TokenMetrics] Failed to unmarshal JSON: %v", err)
		return &extProcPb.ProcessingResponse{
			Response: &extProcPb.ProcessingResponse_ResponseBody{
				ResponseBody: &extProcPb.BodyResponse{},
			},
		}, false
	}

	// usage field existence
	_, exists := responseMap["usage"]
	if !exists {
		log.Printf("[TokenMetrics] No 'usage' field found in response")
		return &extProcPb.ProcessingResponse{
			Response: &extProcPb.ProcessingResponse_ResponseBody{
				ResponseBody: &extProcPb.BodyResponse{},
			},
		}, false
	}

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

	promptTokens := strconv.Itoa(openAIResp.Usage.PromptTokens)
	totalTokens := strconv.Itoa(openAIResp.Usage.TotalTokens)
	completionTokens := strconv.Itoa(openAIResp.Usage.CompletionTokens)

	// headers with token usage
	headers := []*configPb.HeaderValueOption{
		{
			Header: &configPb.HeaderValue{
				Key:   "x-kuadrant-openai-prompt-tokens",
				Value: promptTokens,
			},
			Append: wrapperspb.Bool(false),
		},
		{
			Header: &configPb.HeaderValue{
				Key:   "x-kuadrant-openai-total-tokens",
				Value: totalTokens,
			},
			Append: wrapperspb.Bool(false),
		},
		{
			Header: &configPb.HeaderValue{
				Key:   "x-kuadrant-openai-completion-tokens",
				Value: completionTokens,
			},
			Append: wrapperspb.Bool(false),
		},
	}

	// create response with headers but preserve the body
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

	log.Printf("[TokenMetrics] Added token headers: prompt=%s, completion=%s, total=%s",
		promptTokens, completionTokens, totalTokens)
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
			// pass through headers untouched
			resp = &extProcPb.ProcessingResponse{
				Response: &extProcPb.ProcessingResponse_RequestHeaders{
					RequestHeaders: &extProcPb.HeadersResponse{},
				},
			}

		case *extProcPb.ProcessingRequest_RequestBody:
			// pass body untouched
			resp = &extProcPb.ProcessingResponse{
				Response: &extProcPb.ProcessingResponse_RequestBody{
					RequestBody: &extProcPb.BodyResponse{},
				},
			}

		case *extProcPb.ProcessingRequest_ResponseHeaders:
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

		case *extProcPb.ProcessingRequest_ResponseBody:
			rb := r.ResponseBody
			if !rb.EndOfStream {
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_ResponseBody{
						ResponseBody: &extProcPb.BodyResponse{},
					},
				}
				break
			}

			// Use the shared method to process the response body
			var metricsFound bool
			resp, metricsFound = tm.ProcessResponseBody(rb.Body)

			if !metricsFound {
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_ResponseBody{
						ResponseBody: &extProcPb.BodyResponse{},
					},
				}
			}

		default:
			log.Printf("[TokenMetrics] Received unrecognized request type: %T", r)
			resp = &extProcPb.ProcessingResponse{}
		}

		if err := srv.Send(resp); err != nil {
			log.Printf("[TokenMetrics] Error sending response: %v", err)
			return err
		}
	}
}
