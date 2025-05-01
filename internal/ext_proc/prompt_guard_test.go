package ext_proc_test

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sashabaranov/go-openai"

	"github.com/kuadrant/inferno/internal/ext_proc"
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
