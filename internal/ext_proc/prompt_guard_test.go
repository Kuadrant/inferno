package ext_proc_test

import (
	"context"
	"errors"
	"fmt"
	filterPb "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	extProcPb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	statusPb "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"log"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sashabaranov/go-openai"

	"github.com/kuadrant/inferno/internal/ext_proc"
	"github.com/kuadrant/inferno/internal/testutil"
)

// mockOpenAIClient implements the OpenAIChatCompleter interface for testing
type mockOpenAIClient struct {
	MockResponse    openai.ChatCompletionResponse
	MockError       error
	CapturedRequest openai.ChatCompletionRequest
}

// CreateChatCompletion is the mocked method
func (m *mockOpenAIClient) CreateChatCompletion(ctx context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	m.CapturedRequest = req
	if m.MockError != nil {
		return openai.ChatCompletionResponse{}, m.MockError
	}
	return m.MockResponse, nil
}

var _ = Describe("PromptGuard CheckRisk", func() {
	var (
		pg         *ext_proc.PromptGuard
		mockClient *mockOpenAIClient
		userQuery  string
	)

	BeforeEach(func() {
		mockClient = &mockOpenAIClient{}
		pg = ext_proc.NewPromptGuard(mockClient)
		userQuery = "Is this query risky?"
	})

	riskyToken := "Yes"
	ctx := context.Background()

	// Test the constructor logic when NO client is passed AND no env vars
	Context("when the client is nil (via constructor)", func() {
		BeforeEach(func() {
			pg = ext_proc.NewPromptGuard(nil)
		})
		It("should return false from checkRisk", func() {
			Expect(pg.CheckRisk(ctx, userQuery)).To(BeFalse())
		})
	})

	Context("when the client is initialized (mocked)", func() {

		Context("and the API call fails", func() {
			BeforeEach(func() {
				mockClient.MockError = errors.New("API unavailable")
			})

			It("should return false", func() {
				Expect(pg.CheckRisk(ctx, userQuery)).To(BeFalse())
			})
		})

		Context("and the API call succeeds", func() {
			Context("and the response indicates risk", func() {
				BeforeEach(func() {
					mockClient.MockResponse = openai.ChatCompletionResponse{
						Choices: []openai.ChatCompletionChoice{
							{Message: openai.ChatCompletionMessage{Content: " " + riskyToken + " "}},
						},
					}
				})
				It("should return true", func() {
					Expect(pg.CheckRisk(ctx, userQuery)).To(BeTrue())
				})
			})

			Context("and the response indicates risk (case-insensitive)", func() {
				BeforeEach(func() {
					mockClient.MockResponse = openai.ChatCompletionResponse{
						Choices: []openai.ChatCompletionChoice{
							{Message: openai.ChatCompletionMessage{Content: " yEs \n"}},
						},
					}
				})
				It("should return true", func() {
					Expect(pg.CheckRisk(ctx, userQuery)).To(BeTrue())
				})
			})

			Context("and the response indicates safe", func() {
				BeforeEach(func() {
					mockClient.MockResponse = openai.ChatCompletionResponse{
						Choices: []openai.ChatCompletionChoice{
							{Message: openai.ChatCompletionMessage{Content: "No"}},
						},
					}
				})
				It("should return false", func() {
					Expect(pg.CheckRisk(ctx, userQuery)).To(BeFalse())
				})
			})

			Context("and the response has empty content", func() {
				BeforeEach(func() {
					mockClient.MockResponse = openai.ChatCompletionResponse{
						Choices: []openai.ChatCompletionChoice{
							{Message: openai.ChatCompletionMessage{Content: ""}},
						},
					}
				})
				It("should return false", func() {
					Expect(pg.CheckRisk(ctx, userQuery)).To(BeFalse())
				})
			})

			Context("and the response has no choices", func() {
				BeforeEach(func() {
					mockClient.MockResponse = openai.ChatCompletionResponse{
						Choices: []openai.ChatCompletionChoice{},
					}
				})
				It("should return false", func() {
					Expect(pg.CheckRisk(ctx, userQuery)).To(BeFalse())
				})
			})
		})
	})
})

var _ = Describe("PromptGuard Process", func() {
	var (
		pg         *ext_proc.PromptGuard
		mockClient *mockOpenAIClient
		mockServer *testutil.MockExtProcServer
		riskyToken string
		ginkgoT    GinkgoTInterface
	)

	BeforeEach(func() {
		ginkgoT = GinkgoT()

		mockClient = &mockOpenAIClient{}
		mockServer = testutil.NewMockExtProcServer(10)

		pg = ext_proc.NewPromptGuard(mockClient)

		riskyToken = "Yes"

		ginkgoT.Setenv("DISABLE_PROMPT_RISK_CHECK", "")
		ginkgoT.Setenv("DISABLE_RESPONSE_RISK_CHECK", "")

		go func() {
			defer GinkgoRecover()
			err := pg.Process(mockServer)
			log.Printf("[Test Goroutine] Process finished with error: %v", err)
			select {
			case mockServer.Done <- err:
			default:
				log.Println("[Test Goroutine] Warning: Done channel full or closed.")
			}
			log.Println("[Test Goroutine] Exiting.")
		}()
	})

	AfterEach(func() {
		mockServer.Close()
	})

	waitForResponse := func(timeout time.Duration) *extProcPb.ProcessingResponse {
		select {
		case resp, ok := <-mockServer.Responses:
			if !ok {
				select {
				case err, doneOk := <-mockServer.Done:
					if doneOk && err != nil {
						Fail(fmt.Sprintf("Response channel closed; Process finished unexpectedly with error: %v", err))
					} else {
						Fail("Response channel closed; Process finished unexpectedly (Done chan closed or nil error)")
					}
				default:
					Fail("Response channel closed unexpectedly")
				}
				return nil
			}
			log.Printf("[waitForResponse] Received response.")
			return resp
		case <-time.After(timeout):
			select {
			case err, ok := <-mockServer.Done:
				if ok && err != nil {
					Fail(fmt.Sprintf("Timed out waiting for response after %v; Process finished unexpectedly with error: %v", timeout, err))
				} else {
					Fail(fmt.Sprintf("Timed out waiting for response after %v; Process may have finished cleanly or hung.", timeout))
				}
			default:
				Fail(fmt.Sprintf("Timed out waiting for response after %v", timeout))
			}
			return nil
		}
	}

	waitForDone := func(timeout time.Duration) error {
		select {
		case err, ok := <-mockServer.Done:
			if !ok {
				log.Println("[waitForDone] Done channel closed.")
				return nil
			}
			log.Printf("[waitForDone] Received done signal with error: %v", err)
			return err
		case <-time.After(timeout):
			log.Printf("[waitForDone] Timed out after %v.", timeout)
			return fmt.Errorf("timed out waiting for Process to finish after %v", timeout)
		}
	}

	Context("when processing RequestHeaders", func() {
		It("should send a basic RequestHeaders response", func() {
			mockServer.InjectRequest(&extProcPb.ProcessingRequest{
				Request: &extProcPb.ProcessingRequest_RequestHeaders{},
			})
			resp := waitForResponse(100 * time.Millisecond)
			Expect(resp).NotTo(BeNil())
			Expect(resp.GetResponse()).To(BeAssignableToTypeOf(&extProcPb.ProcessingResponse_RequestHeaders{}))
		})
	})

	Context("when processing RequestBody", func() {
		prompt := "Is this safe?"
		requestBody := &extProcPb.ProcessingRequest_RequestBody{
			RequestBody: &extProcPb.HttpBody{
				Body:        []byte(fmt.Sprintf(`{"prompt": "%s"}`, prompt)),
				EndOfStream: true,
			},
		}

		Context("and prompt is safe", func() {
			BeforeEach(func() {
				mockClient.MockResponse = openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{
						{Message: openai.ChatCompletionMessage{Content: "No"}},
					},
				}
				mockServer.InjectRequest(&extProcPb.ProcessingRequest{Request: requestBody})
			})

			It("should call CheckRisk and allow the request", func() {
				resp := waitForResponse(200 * time.Millisecond)
				Expect(resp).NotTo(BeNil())
				_, ok := resp.GetResponse().(*extProcPb.ProcessingResponse_RequestBody)
				Expect(ok).To(BeTrue(), "Expected RequestBody response type")

				Eventually(func() []openai.ChatCompletionMessage {
					return mockClient.CapturedRequest.Messages
				}).Should(HaveLen(1))
				Expect(mockClient.CapturedRequest.Messages[0].Content).To(Equal(prompt))
			})
		})

		Context("and prompt is risky", func() {
			BeforeEach(func() {
				mockClient.MockResponse = openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{
						{Message: openai.ChatCompletionMessage{Content: riskyToken}},
					},
				}
				mockServer.InjectRequest(&extProcPb.ProcessingRequest{Request: requestBody})
			})

			It("should call CheckRisk and return an ImmediateResponse (403)", func() {
				resp := waitForResponse(200 * time.Millisecond)
				Expect(resp).NotTo(BeNil())

				irResp, ok := resp.GetResponse().(*extProcPb.ProcessingResponse_ImmediateResponse)
				Expect(ok).To(BeTrue(), "Expected ImmediateResponse type")
				Expect(irResp.ImmediateResponse).NotTo(BeNil())
				Expect(irResp.ImmediateResponse.Status.Code).To(Equal(statusPb.StatusCode_Forbidden))
				Expect(string(irResp.ImmediateResponse.Body)).To(ContainSubstring("Prompt blocked"))

				Eventually(func() []openai.ChatCompletionMessage {
					return mockClient.CapturedRequest.Messages
				}).Should(HaveLen(1))
				Expect(mockClient.CapturedRequest.Messages[0].Content).To(Equal(prompt))
			})
		})

		Context("and prompt risk check is disabled", func() {
			BeforeEach(func() {
				ginkgoT.Setenv("DISABLE_PROMPT_RISK_CHECK", "yes")
				mockServer.InjectRequest(&extProcPb.ProcessingRequest{Request: requestBody})
			})

			It("should not call CheckRisk and allow the request", func() {
				resp := waitForResponse(100 * time.Millisecond)
				Expect(resp).NotTo(BeNil())
				_, ok := resp.GetResponse().(*extProcPb.ProcessingResponse_RequestBody)
				Expect(ok).To(BeTrue(), "Expected RequestBody response type")

				// Verify CheckRisk was *not* called
				Consistently(func() []openai.ChatCompletionMessage {
					return mockClient.CapturedRequest.Messages
				}, "50ms", "10ms").Should(BeEmpty())
			})
		})

		Context("with invalid JSON", func() {
			BeforeEach(func() {
				invalidRequestBody := &extProcPb.ProcessingRequest_RequestBody{
					RequestBody: &extProcPb.HttpBody{
						Body:        []byte(`{"prompt": "invalid json`),
						EndOfStream: true,
					},
				}
				mockServer.InjectRequest(&extProcPb.ProcessingRequest{Request: invalidRequestBody})
			})

			It("should return an InvalidArgument error", func() {
				err := waitForDone(100 * time.Millisecond)
				Expect(err).To(HaveOccurred())
				st, ok := status.FromError(err)
				Expect(ok).To(BeTrue(), "Expected gRPC status error")
				Expect(st.Code()).To(Equal(codes.InvalidArgument))
				Expect(st.Message()).To(ContainSubstring("invalid request body"))
			})
		})
	})

	Context("when processing ResponseHeaders", func() {
		It("should send a ResponseHeaders response with ModeOverride", func() {
			mockServer.InjectRequest(&extProcPb.ProcessingRequest{
				Request: &extProcPb.ProcessingRequest_ResponseHeaders{},
			})
			resp := waitForResponse(100 * time.Millisecond)
			Expect(resp).NotTo(BeNil())
			rhResp, ok := resp.GetResponse().(*extProcPb.ProcessingResponse_ResponseHeaders)
			Expect(ok).To(BeTrue(), "Expected ResponseHeaders response type")
			Expect(rhResp.ResponseHeaders).NotTo(BeNil())

			Expect(resp.ModeOverride).NotTo(BeNil())
			Expect(resp.ModeOverride.ResponseHeaderMode).To(Equal(filterPb.ProcessingMode_SKIP))
			Expect(resp.ModeOverride.ResponseBodyMode).To(Equal(filterPb.ProcessingMode_BUFFERED))
		})
	})

	Context("when processing ResponseBody", func() {
		respText := "test"
		responseBodyReq := &extProcPb.ProcessingRequest{
			Request: &extProcPb.ProcessingRequest_ResponseBody{
				ResponseBody: &extProcPb.HttpBody{
					Body:        []byte(fmt.Sprintf(`{"choices": [{"text": "%s"}]}`, respText)),
					EndOfStream: true,
				},
			},
		}

		Context("and output is safe", func() {
			BeforeEach(func() {
				mockClient.MockResponse = openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{
						{Message: openai.ChatCompletionMessage{Content: "No"}}, // Safe
					},
				}
				mockServer.InjectRequest(responseBodyReq)
			})

			It("should call CheckRisk and allow the response", func() {
				resp := waitForResponse(200 * time.Millisecond)
				Expect(resp).NotTo(BeNil())
				_, ok := resp.GetResponse().(*extProcPb.ProcessingResponse_ResponseBody)
				Expect(ok).To(BeTrue(), "Expected ResponseBody response type")

				Eventually(func() []openai.ChatCompletionMessage {
					return mockClient.CapturedRequest.Messages
				}).Should(HaveLen(1))
				Expect(mockClient.CapturedRequest.Messages[0].Content).To(Equal(respText))
			})
		})

		Context("and output is risky", func() {
			BeforeEach(func() {
				mockClient.MockResponse = openai.ChatCompletionResponse{
					Choices: []openai.ChatCompletionChoice{
						{Message: openai.ChatCompletionMessage{Content: riskyToken}}, // Risky
					},
				}
				mockServer.InjectRequest(responseBodyReq)
			})

			It("should call CheckRisk and return an ImmediateResponse (403)", func() {
				resp := waitForResponse(200 * time.Millisecond)
				Expect(resp).NotTo(BeNil())

				irResp, ok := resp.GetResponse().(*extProcPb.ProcessingResponse_ImmediateResponse)
				Expect(ok).To(BeTrue(), "Expected ImmediateResponse type")
				Expect(irResp.ImmediateResponse).NotTo(BeNil())
				Expect(irResp.ImmediateResponse.Status.Code).To(Equal(statusPb.StatusCode_Forbidden))
				Expect(string(irResp.ImmediateResponse.Body)).To(ContainSubstring("LLM output blocked"))

				Eventually(func() []openai.ChatCompletionMessage {
					return mockClient.CapturedRequest.Messages
				}).Should(HaveLen(1))
				Expect(mockClient.CapturedRequest.Messages[0].Content).To(Equal(respText))
			})
		})

		Context("and response risk check is disabled", func() {
			BeforeEach(func() {
				ginkgoT.Setenv("DISABLE_RESPONSE_RISK_CHECK", "yes")
				mockServer.InjectRequest(responseBodyReq)
			})

			It("should not call CheckRisk and allow the response", func() {
				resp := waitForResponse(100 * time.Millisecond)
				Expect(resp).NotTo(BeNil())
				_, ok := resp.GetResponse().(*extProcPb.ProcessingResponse_ResponseBody)
				Expect(ok).To(BeTrue(), "Expected ResponseBody response type")

				Consistently(func() []openai.ChatCompletionMessage {
					return mockClient.CapturedRequest.Messages
				}, "50ms", "10ms").Should(BeEmpty())
			})
		})

		Context("with invalid JSON", func() {
			BeforeEach(func() {
				invalidRespBodyReq := &extProcPb.ProcessingRequest{
					Request: &extProcPb.ProcessingRequest_ResponseBody{
						ResponseBody: &extProcPb.HttpBody{
							Body:        []byte(`{"choices": invalid`),
							EndOfStream: true,
						},
					},
				}
				mockServer.InjectRequest(invalidRespBodyReq)
			})

			It("should return an InvalidArgument error", func() {
				err := waitForDone(100 * time.Millisecond)
				Expect(err).To(HaveOccurred())
				st, ok := status.FromError(err)
				Expect(ok).To(BeTrue())
				Expect(st.Code()).To(Equal(codes.InvalidArgument))
				Expect(st.Message()).To(ContainSubstring("invalid response body"))
			})
		})
	})

	Context("when stream terminates", func() {
		It("should finish cleanly on EOF", func() {
			mockServer.InjectRecvError(io.EOF)
			err := waitForDone(100 * time.Millisecond)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should finish cleanly on Cancel", func() {
			mockServer.InjectRecvError(status.Error(codes.Canceled, "stream canceled by client"))
			err := waitForDone(100 * time.Millisecond)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should return Recv errors", func() {
			expectedErr := status.Error(codes.Unavailable, "network error")
			mockServer.InjectRecvError(expectedErr)
			err := waitForDone(100 * time.Millisecond)
			Expect(err).To(MatchError(expectedErr))
		})

		It("should return Send errors", func() {
			expectedErr := status.Error(codes.Internal, "failed to send")
			mockServer.InjectSendError(expectedErr)

			mockServer.InjectRequest(&extProcPb.ProcessingRequest{
				Request: &extProcPb.ProcessingRequest_RequestHeaders{},
			})

			err := waitForDone(200 * time.Millisecond)
			Expect(err).To(HaveOccurred())
			st, ok := status.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(codes.Unknown))
			Expect(st.Message()).To(ContainSubstring("cannot send stream response"))
		})
	})
})
