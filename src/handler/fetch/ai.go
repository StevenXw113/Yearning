package fetch

import (
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"context"
	"errors"
	"github.com/sashabaranov/go-openai"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const aiRequestTimeout = 60 * time.Second

type AIAssistant struct {
	cc  *openai.Client
	req openai.ChatCompletionRequest
}

func replace(sql, kind string, tables []string) string {
	ai := model.GloAI.Load()
	pp := ai.AdvisorPrompt
	if kind == "text2sql" {
		pp = ai.SQLGenPrompt
	}
	p := strings.ReplaceAll(pp, "{{tables_info}}", strings.Join(tables, "\n"))
	p = strings.ReplaceAll(p, "{{sql}}", sql)
	p = strings.ReplaceAll(p, "{{lang}}", model.C.General.Lang)
	return p
}

func NewAIAgent() (*AIAssistant, error) {
	ai := new(AIAssistant)
	conf := model.GloAI.Load()
	ai.req = openai.ChatCompletionRequest{
		Model:            conf.Model,
		MaxTokens:        conf.MaxTokens,
		Temperature:      conf.Temperature,
		PresencePenalty:  conf.PresencePenalty,
		FrequencyPenalty: conf.FrequencyPenalty,
		TopP:             conf.TopP,
	}
	if conf.APIKey == "" {
		return nil, errors.New("AI 助手未配置 API Key")
	}
	// 只允许 https：明文 http 会把 API Key、表结构与 SQL 语句明文暴露给链路上的中间人
	if conf.BaseUrl != "" {
		u, err := url.Parse(conf.BaseUrl)
		if err != nil || u.Scheme != "https" {
			return nil, errors.New("AI BaseUrl 必须是 https 地址")
		}
	}
	config := openai.DefaultConfig(conf.APIKey)
	config.BaseURL = conf.BaseUrl
	if conf.ProxyURL != "" {
		proxyUrl, err := url.Parse(conf.ProxyURL)
		if err != nil {
			return nil, err
		}
		config.HTTPClient = &http.Client{
			Transport: &http.Transport{Proxy: http.ProxyURL(proxyUrl)},
			Timeout:   aiRequestTimeout,
		}
	}
	ai.cc = openai.NewClientWithConfig(config)
	return ai, nil
}

func (ai *AIAssistant) Messages(messages []openai.ChatCompletionMessage) *AIAssistant {
	ai.req.Messages = messages
	return ai
}

func (ai *AIAssistant) BuildSQLAdvise(prompt *advisorFrom, tables []string, kind string) (string, error) {
	sql, err := factory.GetFingerprint(prompt.SQL)
	if err != nil {
		return "", err
	}
	ai.req.Messages = []openai.ChatCompletionMessage{
		{
			Role:    "system",
			Content: replace(sql, kind, tables),
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), aiRequestTimeout)
	defer cancel()
	resp, e := ai.cc.CreateChatCompletion(ctx, ai.req)
	if e != nil {
		return "", e
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("AI 服务返回内容为空")
	}
	return resp.Choices[0].Message.Content, nil
}

func (ai *AIAssistant) StreamChatCompletion() (*openai.ChatCompletionStream, error) {
	ai.req.Stream = true
	ctx, cancel := context.WithTimeout(context.Background(), aiRequestTimeout)
	defer cancel()
	stream, err := ai.cc.CreateChatCompletionStream(ctx, ai.req)
	if err != nil {
		return nil, err
	}
	return stream, nil
}
