package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/goim/goim/internal/config"
)

// ChatMessage represents a single message in the LLM chat request.
type ChatMessage struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"`
}

// chatRequest is the OpenAI-compatible API request body.
type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
}

// chatResponse is the OpenAI-compatible API response body.
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message ChatMessage `json:"message"`
}

// LLMClient calls an OpenAI-compatible chat completions endpoint.
type LLMClient struct {
	endpoint   string
	model      string
	apiKey     string
	maxTokens  int
	httpClient *http.Client
}

// NewLLMClient creates a new LLMClient from the given configuration.
func NewLLMClient(cfg config.LLMConfig) *LLMClient {
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	return &LLMClient{
		endpoint:  cfg.BaseURL,
		model:     cfg.Model,
		apiKey:    cfg.APIKey,
		maxTokens: maxTokens,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Chat sends a chat completion request to the LLM and returns the assistant response text.
// The endpoint is expected to be an OpenAI-compatible API (POST /v1/chat/completions).
func (c *LLMClient) Chat(ctx context.Context, systemPrompt string, messages []ChatMessage) (string, error) {
	// Build full messages list: system prompt first, then user/assistant history
	fullMessages := make([]ChatMessage, 0, len(messages)+1)
	if systemPrompt != "" {
		fullMessages = append(fullMessages, ChatMessage{Role: "system", Content: systemPrompt})
	}
	fullMessages = append(fullMessages, messages...)

	reqBody := chatRequest{
		Model:     c.model,
		Messages:  fullMessages,
		MaxTokens: c.maxTokens,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	url := c.endpoint + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call LLM API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read LLM response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal LLM response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}

	return chatResp.Choices[0].Message.Content, nil
}
