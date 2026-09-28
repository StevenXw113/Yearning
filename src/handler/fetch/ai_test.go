package fetch

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"Yearning-go/src/model"

	"github.com/sashabaranov/go-openai"
)

func TestCheckBaseURL(t *testing.T) {
	allow := []string{
		"https://api.openai.com",
		"https://api.anthropic.com/v1",
		"http://127.0.0.1:11434",
		"http://localhost:11434",
		"http://10.10.10.30:8000",
		"http://192.168.1.5/v1",
	}
	for _, u := range allow {
		if err := checkBaseURL(u); err != nil {
			t.Errorf("checkBaseURL(%q) 应放行, got %v", u, err)
		}
	}
	deny := []string{
		"http://api.openai.com", // 公网 http 会明文暴露 API Key 与 SQL
		"http://8.8.8.8",
		"api.openai.com", // 缺协议头
		"ftp://127.0.0.1",
	}
	for _, u := range deny {
		if err := checkBaseURL(u); err == nil {
			t.Errorf("checkBaseURL(%q) 应拒绝", u)
		}
	}
}

func TestNormalizeBase(t *testing.T) {
	// 只做去尾斜杠与去空白，版本段由使用方填全（DeepSeek 在根路径、OpenAI 在 /v1）
	cases := map[string]string{
		"https://api.deepseek.com":          "https://api.deepseek.com",
		"https://api.deepseek.com/":         "https://api.deepseek.com",
		"https://api.openai.com/v1":         "https://api.openai.com/v1",
		"http://127.0.0.1:11434/v1":         "http://127.0.0.1:11434/v1",
		"  https://api.anthropic.com/v1   ": "https://api.anthropic.com/v1",
	}
	for in, want := range cases {
		if got := normalizeBase(in); got != want {
			t.Errorf("normalizeBase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAnthropicStreamParse(t *testing.T) {
	sse := `event: message_start
data: {"type":"message_start"}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"SELECT "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"1"}}

event: content_block_stop
data: {"type":"content_block_stop"}

event: message_stop
data: {"type":"message_stop"}

`
	s := newAnthropicStream(io.NopCloser(strings.NewReader(sse)), func() {})
	var got strings.Builder
	for {
		delta, err := s.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		got.WriteString(delta)
	}
	if got.String() != "SELECT 1" {
		t.Errorf("解析结果 = %q, want %q", got.String(), "SELECT 1")
	}
}

func TestAnthropicStreamUpstreamError(t *testing.T) {
	sse := "event: error\ndata: {\"type\":\"error\",\"error\":{\"message\":\"overloaded_error\"}}\n\n"
	s := newAnthropicStream(io.NopCloser(strings.NewReader(sse)), func() {})
	if _, err := s.Recv(); err == nil || !strings.Contains(err.Error(), "overloaded_error") {
		t.Errorf("应返回上游错误, got %v", err)
	}
}

func TestAnthropicBuildSplitsSystem(t *testing.T) {
	a := newAnthropicAgent(modelAI(), "https://api.anthropic.com/v1", nil)
	req := a.build(messagesFixture(), false)
	if req.System != "sys" {
		t.Errorf("system 应提到顶层, got %q", req.System)
	}
	if len(req.Messages) != 2 || req.Messages[0].Role != "user" {
		t.Errorf("messages 应只保留对话角色, got %+v", req.Messages)
	}
	if req.MaxTokens != 1024 {
		t.Errorf("max_tokens 必填，未配置时应兜底 1024, got %d", req.MaxTokens)
	}
}

func TestResponsesStreamParse(t *testing.T) {
	sse := `event: response.created
data: {"type":"response.created"}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"SELECT "}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"1"}

event: response.completed
data: {"type":"response.completed"}

`
	s := newResponsesStream(io.NopCloser(strings.NewReader(sse)), func() {})
	var got strings.Builder
	for {
		delta, err := s.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		got.WriteString(delta)
	}
	if got.String() != "SELECT 1" {
		t.Errorf("解析结果 = %q, want %q", got.String(), "SELECT 1")
	}
}

func TestResponsesStreamFailed(t *testing.T) {
	sse := "event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"message\":\"rate_limit\"}}}\n\n"
	s := newResponsesStream(io.NopCloser(strings.NewReader(sse)), func() {})
	if _, err := s.Recv(); err == nil || !strings.Contains(err.Error(), "rate_limit") {
		t.Errorf("应返回上游错误, got %v", err)
	}
}

func TestResponsesBuildPutsSystemInInput(t *testing.T) {
	a := newResponsesAgent(modelAI(), "https://api.openai.com/v1", nil)
	req := a.build(messagesFixture(), true)
	if len(req.Input) != 3 || req.Input[0].Role != "system" {
		t.Errorf("system 应作为 input 的首条消息, got %+v", req.Input)
	}
	if !req.Stream {
		t.Error("stream 应为 true")
	}
	if req.MaxOutputTokens != 0 {
		t.Errorf("未配置 max_tokens 时不该下发 max_output_tokens, got %d", req.MaxOutputTokens)
	}
}

func TestNewAIAgentRoutesProtocol(t *testing.T) {
	// DeepSeek 与 OpenAI 共用同一实现，只是默认地址/模型不同
	model.GloAI.Store(&model.AI{
		APIKey:   "k",
		BaseUrl:  "https://api.deepseek.com",
		Model:    "deepseek-chat",
		Protocol: "deepseek",
	})
	agent, err := NewAIAgent()
	if err != nil {
		t.Fatalf("deepseek 协议应可创建: %v", err)
	}
	if _, ok := agent.(*openaiAgent); !ok {
		t.Errorf("deepseek 应复用 OpenAI 兼容实现, got %T", agent)
	}

	model.GloAI.Store(&model.AI{APIKey: "k", BaseUrl: "https://x.example.com", Protocol: "bogus"})
	if _, err := NewAIAgent(); err == nil {
		t.Error("未知协议应报错")
	}
}

func TestResponsesTextAndRefusal(t *testing.T) {
	var ok responsesResponse
	_ = json.Unmarshal([]byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"SELECT 1"}]}]}`), &ok)
	got, err := ok.text()
	if err != nil || got != "SELECT 1" {
		t.Errorf("text() = %q, %v; want SELECT 1", got, err)
	}

	// 拒答不是空响应：理由要透出来
	var refused responsesResponse
	_ = json.Unmarshal([]byte(`{"output":[{"type":"message","content":[{"type":"refusal","refusal":"cannot help with that"}]}]}`), &refused)
	if _, err := refused.text(); err == nil || !strings.Contains(err.Error(), "cannot help with that") {
		t.Errorf("拒答应报出理由, got %v", err)
	}
}

func modelAI() model.AI {
	return model.AI{Model: "claude-sonnet-4-5"}
}

func messagesFixture() []openai.ChatCompletionMessage {
	return []openai.ChatCompletionMessage{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
}

func TestMongoPromptSelected(t *testing.T) {
	// 未配置 Mongo 模板：用内置的，且不落到 SQL 模板上
	model.GloAI.Store(&model.AI{AdvisorPrompt: "SQL TEMPLATE {{sql}}", SQLGenPrompt: "GEN TEMPLATE {{sql}}"})
	got := replace("db.runCommand({drop:1})", "advisor", []string{"users(_id, age)"}, true)
	if !strings.Contains(got, "MongoDB command review") {
		t.Errorf("Mongo 场景应用内置 Mongo 模板, got %q", got)
	}
	if !strings.Contains(got, "db.runCommand") || !strings.Contains(got, "users(_id, age)") {
		t.Errorf("占位符应被替换, got %q", got)
	}

	// SQL 场景不受影响
	if got := replace("select 1", "advisor", nil, false); !strings.Contains(got, "SQL TEMPLATE") {
		t.Errorf("SQL 场景应用配置模板, got %q", got)
	}

	// 设置页填了就优先于内置
	model.GloAI.Store(&model.AI{MongoAdvisorPrompt: "CUSTOM MONGO {{sql}}", MongoSQLGenPrompt: "CUSTOM GEN {{sql}}"})
	if got := replace("cmd", "advisor", nil, true); !strings.Contains(got, "CUSTOM MONGO") {
		t.Errorf("应优先用配置的 Mongo 模板, got %q", got)
	}
	if got := replace("cmd", "text2sql", nil, true); !strings.Contains(got, "CUSTOM GEN") {
		t.Errorf("text2sql 应用配置的 Mongo 生成模板, got %q", got)
	}
}
