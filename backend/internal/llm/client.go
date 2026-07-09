package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/goim/goim/internal/config"
)

// ChatMessage 表示LLM聊天请求中的单条消息。
type ChatMessage struct {
	Role    string `json:"role"`    // "system"、"user"、"assistant"
	Content string `json:"content"`
}

// chatRequest 是兼容OpenAI的API请求体。
type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []ChatMessage `json:"messages"`
	MaxTokens int           `json:"max_tokens,omitempty"`
	Stream    bool          `json:"stream,omitempty"`
}

// chatResponse 是兼容OpenAI的API响应体。
type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message          ChatMessage `json:"message"`
	ReasoningContent string      `json:"reasoning_content,omitempty"` // DeepSeek推理字段
}

// ── 流式响应类型 ──

// StreamChunk 表示SSE流式响应中的单个数据块。
type StreamChunk struct {
	Content          string // 增量内容文本
	ReasoningContent string // DeepSeek推理内容增量
	Done             bool   // 流结束时为true
}

// streamChoice 表示流式增量响应中的一个选项。
type streamChoice struct {
Index   int            `json:"index"`
	Delta  streamDelta   `json:"delta"`
	FinishReason string  `json:"finish_reason,omitempty"`
}

type streamDelta struct {
	Role             string `json:"role,omitempty"`
	Content          string `json:"content,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"` // DeepSeek推理增量
}

// streamResponse 表示解析为JSON的单行SSE数据。
type streamResponse struct {
	ID      string         `json:"id,omitempty"`
	Object  string         `json:"object,omitempty"`
	Choices []streamChoice `json:"choices"`
}

// ChunkCallback 在从LLM收到每个流式数据块时被调用。
type ChunkCallback func(chunk StreamChunk)

// LLMClient 调用兼容OpenAI的聊天补全接口。
type LLMClient struct {
	endpoint   string
	model      string
	apiKey     string
	maxTokens  int
	httpClient *http.Client
}

// NewLLMClient 根据给定的配置创建一个新的LLMClient。
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
			Timeout: 120 * time.Second, // 流式传输使用较长的超时时间
		},
	}
}

// Chat 向LLM发送聊天补全请求并返回助手响应文本。
// 接口应为兼容OpenAI的API（POST /v1/chat/completions）。
func (c *LLMClient) Chat(ctx context.Context, systemPrompt string, messages []ChatMessage) (string, error) {
	// 构建完整的消息列表：先放系统提示，然后是用户/助手历史记录
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
		return "", fmt.Errorf("序列化聊天请求失败: %w", err)
	}

	url := c.endpoint + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建HTTP请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用LLM API失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取LLM响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API返回状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("解析LLM响应失败: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("LLM未返回任何选项")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// ChatStream 向LLM发送流式聊天补全请求。
// 它会在收到每个内容增量时调用onChunk，并在流完成后返回完整拼接的响应。
func (c *LLMClient) ChatStream(ctx context.Context, systemPrompt string, messages []ChatMessage, onChunk ChunkCallback) (string, error) {
	// 构建完整的消息列表
	fullMessages := make([]ChatMessage, 0, len(messages)+1)
	if systemPrompt != "" {
		fullMessages = append(fullMessages, ChatMessage{Role: "system", Content: systemPrompt})
	}
	fullMessages = append(fullMessages, messages...)

	reqBody := chatRequest{
		Model:     c.model,
		Messages:  fullMessages,
		MaxTokens: c.maxTokens,
		Stream:    true,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化流式请求失败: %w", err)
	}

	url := c.endpoint + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("创建流式请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("调用LLM流式API失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LLM流式API返回状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	// 逐行读取SSE流
	scanner := bufio.NewScanner(resp.Body)
	// 增大扫描器缓冲区以处理大数据块
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var fullContent strings.Builder
	var fullReasoning strings.Builder

	for scanner.Scan() {
		line := scanner.Text()

		// SSE行以"data: "开头
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// 流结束信号
		if data == "[DONE]" {
			if onChunk != nil {
				onChunk(StreamChunk{Done: true})
			}
			break
		}

		// 解析JSON数据块
		var chunk streamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// 跳过格式错误的行——某些服务商会发送空数据行
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// 追加增量内容
		if delta.Content != "" {
			fullContent.WriteString(delta.Content)
			if onChunk != nil {
				onChunk(StreamChunk{
					Content:          delta.Content,
					ReasoningContent: delta.ReasoningContent,
				})
			}
		}

		// 追加推理内容（DeepSeek特有）
		if delta.ReasoningContent != "" {
			fullReasoning.WriteString(delta.ReasoningContent)
			// 如果内容为空但推理内容存在，仍然发送数据块
			if delta.Content == "" && onChunk != nil {
				onChunk(StreamChunk{
					Content:          delta.Content,
					ReasoningContent: delta.ReasoningContent,
				})
			}
		}

		// 如果存在finish_reason（如"stop"），表示流结束
		if choice.FinishReason != "" && choice.FinishReason != "null" {
			if onChunk != nil {
				onChunk(StreamChunk{Done: true})
			}
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fullContent.String(), fmt.Errorf("读取SSE流失败: %w", err)
	}

	return fullContent.String(), nil
}
