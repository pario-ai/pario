package models

import "encoding/json"

// ChatMessage represents a single message in a chat conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest is an OpenAI-compatible chat completion request.
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

// ChatCompletionResponse is an OpenAI-compatible chat completion response.
type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

// Choice represents a single completion choice.
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// AnthropicRequest is an Anthropic /v1/messages request.
type AnthropicRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	System    string        `json:"system,omitempty"`
	MaxTokens int           `json:"max_tokens"`
	Stream    bool          `json:"stream,omitempty"`
}

// AnthropicContent represents a content block in an Anthropic response.
type AnthropicContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// AnthropicUsage holds token counts from an Anthropic response.
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// AnthropicResponse is an Anthropic /v1/messages response.
type AnthropicResponse struct {
	ID           string             `json:"id"`
	Type         string             `json:"type"`
	Role         string             `json:"role"`
	Model        string             `json:"model"`
	Content      []AnthropicContent `json:"content"`
	StopReason   string             `json:"stop_reason"`
	Usage        *AnthropicUsage    `json:"usage,omitempty"`
}

// ChatCompletionChunk is an OpenAI streaming chunk.
type ChatCompletionChunk struct {
	ID      string        `json:"id"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	Usage   *Usage        `json:"usage,omitempty"`
}

// ChunkChoice is a choice within a streaming chunk.
type ChunkChoice struct {
	Index        int          `json:"index"`
	Delta        ChatMessage  `json:"delta"`
	FinishReason *string      `json:"finish_reason"`
}

// AnthropicStreamEvent represents an Anthropic SSE event.
type AnthropicStreamEvent struct {
	Type    string           `json:"type"`
	Message json.RawMessage  `json:"message,omitempty"`
	Delta   json.RawMessage  `json:"delta,omitempty"`
	Usage   *AnthropicUsage  `json:"usage,omitempty"`
}

// ToUsage converts AnthropicUsage to the standard Usage type.
func (u *AnthropicUsage) ToUsage() *Usage {
	return &Usage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      u.InputTokens + u.OutputTokens,
	}
}
