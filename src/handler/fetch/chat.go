package fetch

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/model"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cookieY/yee"
	"github.com/sashabaranov/go-openai"
	"io"
	"net/http"
)

type message struct {
	Messages []openai.ChatCompletionMessage `json:"messages"`
}

// chatFrame SSE 数据帧负载：v 为增量文本，error 为错误说明（两者互斥）
type chatFrame struct {
	V     string `json:"v,omitempty"`
	Error string `json:"error,omitempty"`
}

// sseDone 流结束标记（与 OpenAI 流式响应的终止帧一致）
const sseDone = "[DONE]"

// sseFrameLine 渲染一帧 SSE 报文。
// 负载用 JSON 编码：回复里的换行若不转义会直接破坏 SSE 分帧（旧实现拼 "data:%s"，
// 既缺 data: 后的空格、又没有换行分隔，浏览器端无法可靠解析）。
func sseFrameLine(f chatFrame) string {
	b, err := json.Marshal(f)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("data: %s\n\n", b)
}

func writeSSEFrame(c yee.Context, f chatFrame) {
	if line := sseFrameLine(f); line != "" {
		fmt.Fprint(c.Response(), line)
		c.Response().Flush()
	}
}

func writeSSEDone(c yee.Context) {
	fmt.Fprintf(c.Response(), "data: %s\n\n", sseDone)
	c.Response().Flush()
}

func AiChat(c yee.Context) error {
	var u message
	if err := c.Bind(&u); err != nil {
		c.Logger().Error(err)
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	if len(u.Messages) == 0 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(errors.New("消息内容不能为空")))
	}

	// 先建客户端与流：未配置 API Key / 地址非法 / 上游拒绝时都还没写 SSE 响应头，
	// 此时可以按普通 JSON 返回，前端才能拿到明确的失败原因（否则只是一个 200 空流）
	cc, err := NewAIAgent()
	if err != nil {
		c.Logger().Error(err)
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	// 系统提示词中的 {{lang}} 必须替换，否则模型收到的指令里是字面量 "{{lang}}"
	system := renderPrompt(model.GloAI.Load().SQLAgentPrompt, "", nil)
	chat := append([]openai.ChatCompletionMessage{{Role: "system", Content: system}}, u.Messages...)

	stream, err := cc.StreamChatCompletion(chat)
	if err != nil {
		c.Logger().Criticalf("ChatCompletionStream error: %v\n", err)
		return c.JSON(http.StatusOK, common.ERR_COMMON_MESSAGE(err))
	}
	defer stream.Close()

	c.Response().Header().Set(yee.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")

	for {
		delta, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			writeSSEDone(c)
			return nil
		}
		// 流中途出错时无法再改状态码，用错误帧告知前端
		if err != nil {
			c.Logger().Errorf("Stream error: %v\n", err)
			writeSSEFrame(c, chatFrame{Error: err.Error()})
			writeSSEDone(c)
			return nil
		}
		if delta == "" {
			continue
		}
		writeSSEFrame(c, chatFrame{V: delta})
	}
}
