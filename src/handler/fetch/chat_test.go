package fetch

import (
	"encoding/json"
	"strings"
	"testing"
)

// 回复内容含换行、中文时，仍必须渲染成**单帧**：SSE 以空行分帧，
// 内容里的裸换行会让浏览器把一帧拆成多帧、甚至丢掉后续内容（旧实现正是如此）。
func TestSSEFrameLineKeepsContentInOneFrame(t *testing.T) {
	const content = "第一行\n\n第二行"
	line := sseFrameLine(chatFrame{V: content})

	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("帧必须以 %q 开头，实际: %q", "data: ", line)
	}
	if !strings.HasSuffix(line, "\n\n") {
		t.Fatalf("帧必须以空行结尾（SSE 分帧要求），实际: %q", line)
	}
	if n := strings.Count(line, "\n\n"); n != 1 {
		t.Fatalf("应只有结尾一处空行，实际出现 %d 处: %q", n, line)
	}

	// 帧必须能被前端逐帧解析回原内容
	payload := strings.TrimSuffix(strings.TrimPrefix(line, "data: "), "\n\n")
	var got chatFrame
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("帧负载应是合法 JSON: %v (%q)", err, payload)
	}
	if got.V != content {
		t.Fatalf("内容往返不一致: want %q, got %q", content, got.V)
	}
}

// 错误帧与文本帧互斥，便于前端区分
func TestSSEFrameLineError(t *testing.T) {
	line := sseFrameLine(chatFrame{Error: "boom"})
	payload := strings.TrimSuffix(strings.TrimPrefix(line, "data: "), "\n\n")
	var got chatFrame
	if err := json.Unmarshal([]byte(payload), &got); err != nil {
		t.Fatalf("帧负载应是合法 JSON: %v", err)
	}
	if got.Error != "boom" || got.V != "" {
		t.Fatalf("错误帧应只带 error 字段，实际: %+v", got)
	}
}
