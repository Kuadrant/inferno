package ext_proc

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"
	"time"

	corePb "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	filterPb "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	extProcPb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	statusPb "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// OpenAIChatCompleter defines the interface needed by PromptGuard
type OpenAIChatCompleter interface {
	CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error)
}

type PromptGuard struct {
	apiKey      string
	baseURL     string
	fullBaseURL string
	modelName   string
	riskyToken  string
	client      OpenAIChatCompleter
}

func NewPromptGuard(client OpenAIChatCompleter) *PromptGuard {
	apiKey := os.Getenv("GUARDIAN_API_KEY")
	baseURL := os.Getenv("GUARDIAN_URL")
	fullBaseURL := baseURL + "/openai/v1"
	modelName := "granite-guardian"
	riskyToken := "Yes"

	if apiKey == "" {
		log.Println("[PromptGuard] Warning: GUARDIAN_API_KEY env var is not set")
	}
	if baseURL == "" {
		log.Println("[PromptGuard] Warning: GUARDIAN_URL env var is not set")
	}

	if client == nil && apiKey != "" && baseURL != "" {
		cfg := openai.DefaultConfig(apiKey)
		cfg.BaseURL = fullBaseURL
		c := openai.NewClientWithConfig(cfg)
		client = c
		log.Printf("[PromptGuard] Initialized with base URL: %s", fullBaseURL)
	}

	return &PromptGuard{
		apiKey:      apiKey,
		baseURL:     baseURL,
		fullBaseURL: fullBaseURL,
		modelName:   modelName,
		riskyToken:  riskyToken,
		client:      client,
	}
}

func (pg *PromptGuard) CheckRisk(ctx context.Context, userQuery string) bool {
	if pg.client == nil {
		log.Println("[PromptGuard] Client not initialized, skipping risk check")
		return false
	}

	log.Printf("👮‍♀️ [Guardian] Checking risk on: '%s'\n", userQuery)
	log.Printf("→ Sending to: %s/chat/completions with model '%s'\n", pg.fullBaseURL, pg.modelName)

	resp, err := pg.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: pg.modelName,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: userQuery,
			},
		},
		Temperature: 0.01,
		MaxTokens:   50,
	})
	if err != nil {
		if status.Code(err) == codes.Canceled {
			log.Println("[PromptGuard] Risk check canceled by context, returning safe")
			return false
		}
		log.Printf("[PromptGuard] Risk model call failed: %v", err)
		return false
	}

	if len(resp.Choices) == 0 {
		log.Println("[PromptGuard] No choices in response")
		return false
	}
	result := strings.TrimSpace(resp.Choices[0].Message.Content)
	log.Printf("🛡️ Risk Model Response: %s\n", result)

	return strings.EqualFold(result, pg.riskyToken)
}

func (pg *PromptGuard) Process(srv extProcPb.ExternalProcessor_ProcessServer) error {
	log.Println("[PromptGuard] Starting processing loop")
	for {
		req, err := srv.Recv()
		if err == io.EOF {
			log.Println("[PromptGuard] Received EOF, terminating processing loop")
			return nil
		}
		if err != nil {
			if status.Code(err) == codes.Canceled {
				log.Println("[PromptGuard] Stream cancelled, finishing up")
				return nil
			}
			log.Printf("[PromptGuard] Error receiving request: %v", err)
			return err
		}

		log.Printf("[PromptGuard] Received request: %+v", req)

		var resp *extProcPb.ProcessingResponse

		switch r := req.Request.(type) {
		case *extProcPb.ProcessingRequest_RequestHeaders:
			log.Println("[PromptGuard] Processing RequestHeaders")
			// pass through headers untouched
			resp = &extProcPb.ProcessingResponse{
				Response: &extProcPb.ProcessingResponse_RequestHeaders{
					RequestHeaders: &extProcPb.HeadersResponse{},
				},
			}
			log.Println("[PromptGuard] RequestHeaders processed, passing through response unchanged")

		case *extProcPb.ProcessingRequest_RequestBody:
			log.Println("[PromptGuard] Processing RequestBody")

			bodyStr := string(r.RequestBody.Body)
			log.Printf("[PromptGuard] Request body: %s", bodyStr)

			var bodyMap map[string]interface{}
			if err := json.Unmarshal([]byte(bodyStr), &bodyMap); err != nil {
				log.Printf("[PromptGuard] Failed to parse request body: %v", err)
				return status.Errorf(codes.InvalidArgument, "invalid request body: %v", err)
			}

			prompt, _ := bodyMap["prompt"].(string)
			log.Printf("[PromptGuard] Extracted prompt: %s", prompt)

			if os.Getenv("DISABLE_PROMPT_RISK_CHECK") == "yes" {
				log.Println("[PromptGuard] Prompt risk check disabled via env var, allowing request")
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_RequestBody{
						RequestBody: &extProcPb.BodyResponse{},
					},
				}
			} else {
				// use independent timeout so we don't get canceled by srv.Context
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if pg.CheckRisk(ctx, prompt) {
					log.Println("[PromptGuard] Risky prompt detected, returning 403")

					resp = &extProcPb.ProcessingResponse{
						Response: &extProcPb.ProcessingResponse_ImmediateResponse{
							ImmediateResponse: &extProcPb.ImmediateResponse{
								Status: &statusPb.HttpStatus{
									Code: statusPb.StatusCode_Forbidden,
								},
								Body: []byte(`{"error":"Prompt blocked by content policy"}`),
								Headers: &extProcPb.HeaderMutation{
									SetHeaders: []*corePb.HeaderValueOption{
										{
											Header: &corePb.HeaderValue{
												Key:   "Content-Type",
												Value: "application/json",
											},
										},
									},
								},
							},
						},
					}
				} else {
					log.Println("[PromptGuard] Prompt safe, allowing request")
					resp = &extProcPb.ProcessingResponse{
						Response: &extProcPb.ProcessingResponse_RequestBody{
							RequestBody: &extProcPb.BodyResponse{},
						},
					}
				}
			}

		case *extProcPb.ProcessingRequest_ResponseHeaders:
			log.Println("[PromptGuard] Processing ResponseHeaders, instructing Envoy to buffer response body")
			resp = &extProcPb.ProcessingResponse{
				Response: &extProcPb.ProcessingResponse_ResponseHeaders{
					ResponseHeaders: &extProcPb.HeadersResponse{},
				},
				ModeOverride: &filterPb.ProcessingMode{
					ResponseHeaderMode: filterPb.ProcessingMode_SKIP,
					ResponseBodyMode:   filterPb.ProcessingMode_BUFFERED,
				},
			}
			log.Println("[PromptGuard] ResponseHeaders processed, buffering response body")

		case *extProcPb.ProcessingRequest_ResponseBody:
			log.Println("[PromptGuard] Processing ResponseBody")
			rb := r.ResponseBody
			log.Printf("[PromptGuard] ResponseBody received, EndOfStream: %v", rb.EndOfStream)

			if !rb.EndOfStream {
				log.Println("[PromptGuard] ResponseBody not complete, continuing to buffer")
				break
			}

			bodyStr := string(rb.Body)
			log.Printf("[PromptGuard] Full response body: %s", bodyStr)

			var respData map[string]interface{}
			if err := json.Unmarshal(rb.Body, &respData); err != nil {
				log.Printf("[PromptGuard] Failed to parse response body: %v", err)
				return status.Errorf(codes.InvalidArgument, "invalid response body: %v", err)
			}

			var generated string
			choices, ok := respData["choices"].([]interface{})
			if ok && len(choices) > 0 {
				first, _ := choices[0].(map[string]interface{})
				generated, _ = first["text"].(string)
			}
			log.Printf("[PromptGuard] Extracted response text: %s", generated)

			if os.Getenv("DISABLE_RESPONSE_RISK_CHECK") == "yes" {
				log.Println("[PromptGuard] Response risk check disabled via env var, allowing response")
				resp = &extProcPb.ProcessingResponse{
					Response: &extProcPb.ProcessingResponse_ResponseBody{
						ResponseBody: &extProcPb.BodyResponse{},
					},
				}
			} else {
				// use independent timeout so we don't get canceled by srv.Context
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if pg.CheckRisk(ctx, generated) {
					log.Println("[PromptGuard] Risky LLM output detected, blocking response")
					resp = &extProcPb.ProcessingResponse{
						Response: &extProcPb.ProcessingResponse_ImmediateResponse{
							ImmediateResponse: &extProcPb.ImmediateResponse{
								Status: &statusPb.HttpStatus{
									Code: statusPb.StatusCode_Forbidden,
								},
								Body: []byte(`{"error":"LLM output blocked by safety filter"}`),
								Headers: &extProcPb.HeaderMutation{
									SetHeaders: []*corePb.HeaderValueOption{
										{
											Header: &corePb.HeaderValue{
												Key:   "Content-Type",
												Value: "application/json",
											},
										},
									},
								},
							},
						},
					}
				} else {
					log.Println("[PromptGuard] LLM output safe, allowing response")
					resp = &extProcPb.ProcessingResponse{
						Response: &extProcPb.ProcessingResponse_ResponseBody{
							ResponseBody: &extProcPb.BodyResponse{},
						},
					}
				}
			}

		default:
			log.Printf("[PromptGuard] Received unrecognized request type: %+v", r)
			resp = &extProcPb.ProcessingResponse{}
		}

		if err := srv.Send(resp); err != nil {
			log.Printf("[PromptGuard] Error sending response: %v", err)
			if status.Code(err) == codes.Canceled {
				log.Println("[PromptGuard] Stream canceled, exiting cleanly")
				return nil
			}
			return status.Errorf(codes.Unknown, "cannot send stream response: %v", err)
		}
	}
}
