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

// responsesAgent 走 OpenAI Responses API（/v1/responses）。
// 与 Chat Completions 的差异：消息字段是 input、输出上限是 max_output_tokens、
// 流式事件为 response.output_text.delta。
type responsesAgent struct {
	conf   model.AI
	url    string
	client *http.Client
}

func newResponsesAgent(conf model.AI, base string, client *http.Client) *responsesAgent {
	return &responsesAgent{conf: conf, url: base + "/responses", client: client}
}

type responsesRequest struct {
	Model           string        `json:"model"`
	Input           []chatMessage `json:"input"`
	MaxOutputTokens int           `json:"max_output_tokens,omitempty"`
	Temperature     *float32      `json:"temperature,omitempty"`
	TopP            *float32      `json:"top_p,omitempty"`
	Stream          bool          `json:"stream,omitempty"`
}

type responsesResponse struct {
	Output []struct {
		Type    string `json:"type"`
		Content []struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Refusal string `json:"refusal"`
		} `json:"content"`
	} `json:"output"`
}

// build 组装 Responses 报文：system 与对话消息一样放进 input
func (a *responsesAgent) build(messages []openai.ChatCompletionMessage, stream bool) responsesRequest {
	req := responsesRequest{Model: a.conf.Model, Stream: stream, MaxOutputTokens: a.conf.MaxTokens}
	// 采样参数为 0 视为「未配置」而不下发
	if a.conf.Temperature > 0 {
		t := a.conf.Temperature
		req.Temperature = &t
	}
	if a.conf.TopP > 0 {
		p := a.conf.TopP
		req.TopP = &p
	}
	for _, m := range messages {
		req.Input = append(req.Input, chatMessage{Role: m.Role, Content: m.Content})
	}
	return req
}

func (a *responsesAgent) post(ctx context.Context, req responsesRequest) (*http.Response, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.conf.APIKey)
	return a.client.Do(httpReq)
}

func (a *responsesAgent) BuildSQLAdvise(prompt *advisorFrom, tables []string, kind string) (string, error) {
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
	var out responsesResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return "", err
	}
	return out.text()
}

// text 抽取响应正文：正常取 output_text；模型拒答时把理由作为错误抛出
// （拒答不是空响应，只报「内容为空」会让人看不出原因）。
func (r responsesResponse) text() (string, error) {
	var sb, refusal strings.Builder
	for _, item := range r.Output {
		for _, c := range item.Content {
			switch c.Type {
			case "output_text":
				sb.WriteString(c.Text)
			case "refusal":
				refusal.WriteString(c.Refusal)
			}
		}
	}
	if sb.Len() == 0 {
		if refusal.Len() > 0 {
			return "", fmt.Errorf("AI 服务拒绝回答: %s", refusal.String())
		}
		return "", errors.New("AI 服务返回内容为空")
	}
	return sb.String(), nil
}

func (a *responsesAgent) StreamChatCompletion(messages []openai.ChatCompletionMessage) (AIStream, error) {
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
	return newResponsesStream(resp.Body, cancel), nil
}

type responsesEvent struct {
	Type    string `json:"type"`
	Delta   string `json:"delta"`
	Message string `json:"message"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
	Response *struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
}

// newResponsesStream 按 response.output_text.delta 事件抽取增量文本
func newResponsesStream(body io.ReadCloser, cancel func()) *sseStream {
	return newSSEStream(body, cancel, func(data []byte) (string, error) {
		var ev responsesEvent
		if err := json.Unmarshal(data, &ev); err != nil {
			return "", nil // 非 JSON 帧（心跳等）跳过
		}
		switch ev.Type {
		case "response.output_text.delta":
			return ev.Delta, nil
		case "response.completed", "response.incomplete":
			return "", io.EOF
		case "response.failed":
			if ev.Response != nil && ev.Response.Error != nil {
				return "", errors.New(ev.Response.Error.Message)
			}
			return "", errors.New("AI 服务返回失败")
		case "error":
			if ev.Error != nil {
				return "", errors.New(ev.Error.Message)
			}
			if ev.Message != "" {
				return "", errors.New(ev.Message)
			}
			return "", errors.New("AI 服务返回错误")
		}
		return "", nil
	})
}
