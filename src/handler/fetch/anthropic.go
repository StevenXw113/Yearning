package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"Yearning-go/src/model"

	"github.com/sashabaranov/go-openai"
)

// anthropicVersion 是 Messages API 要求的版本头
const anthropicVersion = "2023-06-01"

// anthropicAgent 走 Anthropic Messages API（Claude）。
// 与 OpenAI 协议的差异：鉴权用 x-api-key 而非 Bearer、system 是顶层字段而非消息、
// max_tokens 必填、不支持 presence/frequency penalty、流式事件是 content_block_delta。
type anthropicAgent struct {
	conf   model.AI
	url    string
	client *http.Client
}

func newAnthropicAgent(conf model.AI, base string, client *http.Client) *anthropicAgent {
	return &anthropicAgent{conf: conf, url: base + "/messages", client: client}
}

type anthropicRequest struct {
	Model       string        `json:"model"`
	MaxTokens   int           `json:"max_tokens"`
	System      string        `json:"system,omitempty"`
	Messages    []chatMessage `json:"messages"`
	Temperature *float32      `json:"temperature,omitempty"`
	TopP        *float32      `json:"top_p,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// build 把统一的 OpenAI 风格消息转成 Anthropic 报文：system 提到顶层，其余按原角色传递
func (a *anthropicAgent) build(messages []openai.ChatCompletionMessage, stream bool) anthropicRequest {
	req := anthropicRequest{Model: a.conf.Model, MaxTokens: a.conf.MaxTokens, Stream: stream}
	if req.MaxTokens <= 0 {
		req.MaxTokens = 1024 // Anthropic 必填；配置留空时给一个安全值
	}
	// 采样参数为 0 视为「未配置」而不下发，避免把 0 当成确定性输出传给模型
	if a.conf.Temperature > 0 {
		t := a.conf.Temperature
		req.Temperature = &t
	}
	if a.conf.TopP > 0 {
		p := a.conf.TopP
		req.TopP = &p
	}
	for _, m := range messages {
		if m.Role == "system" {
			req.System = m.Content
			continue
		}
		req.Messages = append(req.Messages, chatMessage{Role: m.Role, Content: m.Content})
	}
	return req
}

func (a *anthropicAgent) post(ctx context.Context, req anthropicRequest) (*http.Response, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.conf.APIKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	return a.client.Do(httpReq)
}

func (a *anthropicAgent) BuildSQLAdvise(prompt *advisorFrom, tables []string, kind string) (string, error) {
	system, err := adviseSystemPrompt(prompt, tables, kind)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), aiRequestTimeout)
	defer cancel()
	resp, err := a.post(ctx, a.build([]openai.ChatCompletionMessage{{Role: "system", Content: system}}, false))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("AI 服务返回 %d: %s", resp.StatusCode, apiErrorBody(resp))
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var out anthropicResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, c := range out.Content {
		if c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	if sb.Len() == 0 {
		return "", errors.New("AI 服务返回内容为空")
	}
	return sb.String(), nil
}

func (a *anthropicAgent) StreamChatCompletion(messages []openai.ChatCompletionMessage) (AIStream, error) {
	// ctx 的生命周期跟着流走：在函数内 defer cancel 会让流刚建立就被取消
	ctx, cancel := context.WithTimeout(context.Background(), aiStreamTimeout)
	resp, err := a.post(ctx, a.build(messages, true))
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		msg := apiErrorBody(resp)
		cancel()
		return nil, fmt.Errorf("AI 服务返回 %d: %s", resp.StatusCode, msg)
	}
	return newAnthropicStream(resp.Body, cancel), nil
}

type anthropicEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// newAnthropicStream 按 content_block_delta 事件抽取增量文本
func newAnthropicStream(body io.ReadCloser, cancel func()) *sseStream {
	return newSSEStream(body, cancel, func(data []byte) (string, error) {
		var ev anthropicEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return "", nil // 非 JSON 帧（心跳等）跳过
		}
		if ev.Error != nil {
			return "", errors.New(ev.Error.Message)
		}
		switch ev.Type {
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" {
				return ev.Delta.Text, nil
			}
		case "message_stop":
			return "", io.EOF
		}
		return "", nil
	})
}
