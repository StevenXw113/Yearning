package fetch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"

	"github.com/sashabaranov/go-openai"
)

const (
	// aiRequestTimeout 非流式请求（SQL 优化 / SQL 生成）的超时
	aiRequestTimeout = 60 * time.Second
	// aiStreamTimeout 流式对话的整体超时：模型长回答会持续数分钟，不能沿用 60s
	aiStreamTimeout = 10 * time.Minute
)

// AIStream 各协议统一的流式读取接口：Recv 返回增量文本，流结束时返回 io.EOF。
type AIStream interface {
	Recv() (string, error)
	Close() error
}

// AIAssistant 与协议无关的助手接口
type AIAssistant interface {
	BuildSQLAdvise(prompt *advisorFrom, tables []string, kind string) (string, error)
	StreamChatCompletion(messages []openai.ChatCompletionMessage) (AIStream, error)
}

// NewAIAgent 按配置的接入协议创建助手。
// protocol 为空按 openai（OpenAI 兼容：OpenAI / DeepSeek / Ollama / vLLM 等）处理，兼容历史配置。
func NewAIAgent() (AIAssistant, error) {
	conf := model.GloAI.Load()
	if conf.APIKey == "" {
		return nil, errors.New("AI 助手未配置 API Key")
	}
	if conf.BaseUrl == "" {
		return nil, errors.New("AI 助手未配置接口地址")
	}
	if err := checkBaseURL(conf.BaseUrl); err != nil {
		return nil, err
	}
	client, err := newAIHTTPClient(conf)
	if err != nil {
		return nil, err
	}
	base := normalizeBase(conf.BaseUrl)
	switch strings.ToLower(strings.TrimSpace(conf.Protocol)) {
	case "", "openai", "deepseek":
		// DeepSeek 提供 OpenAI 兼容接口（/chat/completions），与 OpenAI 复用同一实现
		return newOpenAIAgent(*conf, base, client), nil
	case "anthropic":
		return newAnthropicAgent(*conf, base, client), nil
	case "responses":
		return newResponsesAgent(*conf, base, client), nil
	default:
		return nil, fmt.Errorf("不支持的 AI 接入协议: %s", conf.Protocol)
	}
}

// checkBaseURL 校验接口地址：https 一律放行；http 只允许本机与内网地址——
// 明文链路上的 API Key、表结构与 SQL 会被中间人直接看到，公网必须使用 https。
func checkBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return errors.New("AI 接口地址格式不正确")
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLocalHost(u.Hostname()) {
			return nil
		}
		return errors.New("接口地址使用 http 时仅允许本机或内网地址（localhost / 127.0.0.1 / 10.x / 172.16-31.x / 192.168.x / ::1），公网请使用 https")
	default:
		return errors.New("AI 接口地址需以 http:// 或 https:// 开头")
	}
}

// isLocalHost 判断主机名是否为本机或内网地址
func isLocalHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

// normalizeBase 只去掉尾部斜杠：接口地址由使用方填全（含版本段）。
// 各家的版本路径并不统一——OpenAI / Ollama 在 /v1 下，DeepSeek 的 OpenAI 兼容端点在根路径，
// 自动补 /v1 必然在一方出错，因此不做猜测。
func normalizeBase(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// newAIHTTPClient 构造请求客户端。不设 http.Client.Timeout：它会把流式响应的
// 读取一起掐断，超时统一由各请求的 context 控制。
func newAIHTTPClient(conf *model.AI) (*http.Client, error) {
	transport := &http.Transport{}
	if conf.ProxyURL != "" {
		proxyUrl, err := url.Parse(conf.ProxyURL)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(proxyUrl)
	}
	return &http.Client{Transport: transport}, nil
}

// adviseSystemPrompt 渲染 SQL 优化/生成的 system 提示词
func adviseSystemPrompt(prompt *advisorFrom, tables []string, kind string) (string, error) {
	sql, err := factory.GetFingerprint(prompt.SQL)
	if err != nil {
		return "", err
	}
	return replace(sql, kind, tables, prompt.Mongo), nil
}

// truncate 截断上游返回的正文，避免把整段响应塞进报错信息
func truncate(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "..."
}

// ---------- 各协议共用 ----------

// chatMessage 各协议共用的对话消息
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// sseStream SSE 流读取骨架：协议实现只需给出「单帧 JSON → 增量文本」的解析函数，
// 返回空串表示该帧不含内容（跳过），返回 io.EOF 表示流正常结束。
type sseStream struct {
	scanner *bufio.Scanner
	closer  io.Closer
	cancel  func()
	handle  func([]byte) (string, error)
}

func newSSEStream(body io.ReadCloser, cancel func(), handle func([]byte) (string, error)) *sseStream {
	sc := bufio.NewScanner(body)
	// SSE 单帧可能较长（长段落一次性返回），放宽 scanner 默认的 64KB 上限
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	return &sseStream{scanner: sc, closer: body, cancel: cancel, handle: handle}
}

func (s *sseStream) Recv() (string, error) {
	for s.scanner.Scan() {
		data, ok := strings.CutPrefix(s.scanner.Text(), "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "" {
			continue
		}
		text, err := s.handle([]byte(data))
		if err != nil {
			return "", err
		}
		if text != "" {
			return text, nil
		}
	}
	if err := s.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

func (s *sseStream) Close() error {
	s.cancel()
	return s.closer.Close()
}

// apiErrorBody 读取并关闭错误响应，优先取 {"error":{"message":"..."}} 中的说明
func apiErrorBody(resp *http.Response) string {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	var out struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(b, &out) == nil && out.Error != nil && out.Error.Message != "" {
		return out.Error.Message
	}
	return truncate(string(b), 200)
}

// renderPrompt 填充提示词模板中的占位符（模板由调用方提供）
func renderPrompt(tpl, sql string, tables []string) string {
	p := strings.ReplaceAll(tpl, "{{tables_info}}", strings.Join(tables, "\n"))
	p = strings.ReplaceAll(p, "{{sql}}", sql)
	return strings.ReplaceAll(p, "{{lang}}", model.C.General.Lang)
}

func replace(sql, kind string, tables []string, mongo bool) string {
	ai := model.GloAI.Load()
	pp := ai.AdvisorPrompt
	if kind == "text2sql" {
		pp = ai.SQLGenPrompt
	}
	if mongo {
		// MongoDB 命令不是 SQL：关注点不同（空 filter、全集合扫描、索引缺失），
		// 用 Mongo 模板；设置页填过就用设置页的，否则用内置默认
		pp = mongoPrompt(ai, kind)
	}
	return renderPrompt(pp, sql, tables)
}

// mongoPrompt 挑选 MongoDB 场景的提示词模板：设置页配置优先，未配置用内置默认
func mongoPrompt(ai *model.AI, kind string) string {
	if kind == "text2sql" {
		if ai.MongoSQLGenPrompt != "" {
			return ai.MongoSQLGenPrompt
		}
		return defaultMongoSQLGenPrompt
	}
	if ai.MongoAdvisorPrompt != "" {
		return ai.MongoAdvisorPrompt
	}
	return defaultMongoAdvisorPrompt
}

// MongoDB 场景的内置提示词模板（设置页留空时使用）。
// 与 SQL 模板的差别在关注点：Mongo 命令的主要风险是空 filter、全集合扫描、
// 缺失索引与不可逆操作，而不是 JOIN / 子查询这类 SQL 议题。
const defaultMongoAdvisorPrompt = `
MongoDB command review assistant

You are a senior MongoDB DBA. Review the MongoDB change command below and:
- point out the risks: whole-collection writes (empty or missing filter), unbounded scans, missing index for the filter, irreversible operations
- propose a safer version with a narrower filter, a batch size, and the index that would support it
- keep the command as MongoDB extended JSON, e.g. {"update":"users","updates":[{"q":{...},"u":{...}}]}

Collections and their fields:

{{tables_info}}

Command:

{{sql}}

Reply Language: {{lang}}
`

const defaultMongoSQLGenPrompt = `
MongoDB command generation assistant

Now you will play the role of a professional DBA and generate the corresponding MongoDB command (MongoDB extended JSON) from the user's description. Always carry an explicit non-empty filter, and state which index that filter needs.

Collections and their fields: {{tables_info}}

Requirement: {{sql}}

Use the markdown format

Reply Language: {{lang}}
`

// ---------- OpenAI 兼容协议 ----------

type openaiAgent struct {
	cc  *openai.Client
	req openai.ChatCompletionRequest
}

func newOpenAIAgent(conf model.AI, base string, client *http.Client) *openaiAgent {
	cfg := openai.DefaultConfig(conf.APIKey)
	cfg.BaseURL = base
	cfg.HTTPClient = client
	return &openaiAgent{
		cc: openai.NewClientWithConfig(cfg),
		req: openai.ChatCompletionRequest{
			Model:            conf.Model,
			MaxTokens:        conf.MaxTokens,
			Temperature:      conf.Temperature,
			PresencePenalty:  conf.PresencePenalty,
			FrequencyPenalty: conf.FrequencyPenalty,
			TopP:             conf.TopP,
		},
	}
}

func (a *openaiAgent) BuildSQLAdvise(prompt *advisorFrom, tables []string, kind string) (string, error) {
	system, err := adviseSystemPrompt(prompt, tables, kind)
	if err != nil {
		return "", err
	}
	req := a.req
	req.Messages = []openai.ChatCompletionMessage{{Role: "system", Content: system}}
	ctx, cancel := context.WithTimeout(context.Background(), aiRequestTimeout)
	defer cancel()
	resp, err := a.cc.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("AI 服务返回内容为空")
	}
	return resp.Choices[0].Message.Content, nil
}

func (a *openaiAgent) StreamChatCompletion(messages []openai.ChatCompletionMessage) (AIStream, error) {
	req := a.req
	req.Messages = messages
	req.Stream = true
	// ctx 的生命周期跟着流走：在函数内 defer cancel 会让流刚建立就被取消
	ctx, cancel := context.WithTimeout(context.Background(), aiStreamTimeout)
	s, err := a.cc.CreateChatCompletionStream(ctx, req)
	if err != nil {
		cancel()
		return nil, err
	}
	return &openaiStream{s: s, cancel: cancel}, nil
}

type openaiStream struct {
	s      *openai.ChatCompletionStream
	cancel func()
}

func (s *openaiStream) Recv() (string, error) {
	resp, err := s.s.Recv()
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	return resp.Choices[0].Delta.Content, nil
}

func (s *openaiStream) Close() error {
	err := s.s.Close()
	s.cancel()
	return err
}
