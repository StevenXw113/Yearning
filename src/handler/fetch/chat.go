package fetch

import (
	"Yearning-go/src/model"
	"errors"
	"fmt"
	"github.com/cookieY/yee"
	"github.com/sashabaranov/go-openai"
	"io"
)

type message struct {
	Messages []openai.ChatCompletionMessage `json:"messages"`
}

func AiChat(c yee.Context) error {

	c.Response().Header().Set(yee.HeaderContentType, "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")

	var u message
	var chat []openai.ChatCompletionMessage
	if err := c.Bind(&u); err != nil {
		c.Logger().Error(err)
		return c.JSON(200, "Illegal")
	}
	chat = append(chat, openai.ChatCompletionMessage{Role: "system", Content: model.GloAI.Load().SQLAgentPrompt})
	chat = append(chat, u.Messages...)

	cc, err := NewAIAgent()
	if err != nil {
		c.Logger().Error(err)
		return err
	}
	stream, err := cc.Messages(chat).StreamChatCompletion()
	if err != nil {
		c.Logger().Criticalf("ChatCompletionStream error: %v\n", err)
		return err
	}
	defer stream.Close()

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			c.Logger().Errorf("Stream error: %v\n", err)
			return nil
		}

		if len(response.Choices) == 0 {
			continue
		}

		fmt.Fprintf(c.Response(), "data:%s", response.Choices[0].Delta.Content)
		c.Response().Flush()
	}
}
