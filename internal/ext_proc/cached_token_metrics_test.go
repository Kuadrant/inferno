package ext_proc

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("TokenMetricsWithCache", func() {
	It("should extract token metrics from cached response in processor", func() {
		responseWithMetrics := []byte(`{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 1677652288,
			"model": "gpt-3.5-turbo",
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Hello, how can I help you today?"
				},
				"index": 0,
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 50,
				"completion_tokens": 75,
				"total_tokens": 125
			}
		}`)

		headers := ExtractTokenMetricsHeaders(responseWithMetrics)
		Expect(headers).ToNot(BeNil(), "Should extract token metrics headers from response")
		Expect(headers).To(HaveLen(3), "Should have 3 token metric headers")

		headerMap := make(map[string]string)
		for _, h := range headers {
			headerMap[h.Header.Key] = h.Header.Value
		}

		Expect(headerMap["x-kuadrant-openai-prompt-tokens"]).To(Equal("50"), "Prompt tokens should be 50")
		Expect(headerMap["x-kuadrant-openai-completion-tokens"]).To(Equal("75"), "Completion tokens should be 75")
		Expect(headerMap["x-kuadrant-openai-total-tokens"]).To(Equal("125"), "Total tokens should be 125")
	})

	It("should handle responses with no token metrics", func() {
		responseWithoutMetrics := []byte(`{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"created": 1677652288,
			"model": "gpt-3.5-turbo",
			"choices": [{
				"message": {
					"role": "assistant",
					"content": "Hello, how can I help you today?"
				},
				"index": 0,
				"finish_reason": "stop"
			}]
		}`)

		headers := ExtractTokenMetricsHeaders(responseWithoutMetrics)
		Expect(headers).To(BeNil(), "Should not extract headers from response without metrics")
	})
})
