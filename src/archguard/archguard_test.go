// Package archguard 主程序的架构护栏测试（对齐引擎侧 internal/archguard 的思路）：
// 用测试锁死跨端契约与代码纪律，违反即 go test 失败，而不是上线后踩坑。
package archguard

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"Yearning-go/src/model"
)

// frontendRoot 从 src/archguard 向上定位 front/ 目录（仓库布局固定）。
func frontendRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(dir + "/front/src/types/index.ts"); err == nil {
			return dir
		}
		parent := parentDir(dir)
		if parent == dir {
			t.Fatal("向上未找到 front/src/types/index.ts")
		}
		dir = parent
	}
}

func parentDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return p
}

// readOrderStateEnum 解析前端 OrderState 枚举：成员名 → 值（支持显式赋值与自增）。
func readOrderStateEnum(t *testing.T) map[string]int {
	t.Helper()
	data, err := os.ReadFile(frontendRoot(t) + "/front/src/types/index.ts")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`export enum OrderState \{([^}]*)\}`).FindSubmatch(data)
	if m == nil {
		t.Fatal("types/index.ts 中未找到 OrderState 枚举")
	}
	out := map[string]int{}
	next := 0
	for _, line := range strings.Split(string(m[1]), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimSuffix(line, ",")
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		name, val := line, next
		if kv := strings.SplitN(line, "=", 2); len(kv) == 2 {
			name = strings.TrimSpace(kv[0])
			v, err := strconv.Atoi(strings.TrimSpace(kv[1]))
			if err != nil {
				t.Fatalf("OrderState 成员 %q 的值不是十进制整数", line)
			}
			val = v
		}
		out[name] = val
		next = val + 1
	}
	return out
}

// TestOrderStateEnumContract 工单状态前后端契约：Go 的 model.OrderStatus* 常量
// 必须与前端 OrderState 枚举逐一对齐。历史上两端各自手抄、无任何校验——
// 状态值漂移会让「执行中」显示成别的状态、守卫条件静默错位。
func TestOrderStateEnumContract(t *testing.T) {
	fe := readOrderStateEnum(t)
	pairs := []struct {
		goName  string
		goValue int
		feName  string
	}{
		{"OrderStatusRejected", model.OrderStatusRejected, "REJECT"},
		{"OrderStatusFinished", model.OrderStatusFinished, "SUCCESS"},
		{"OrderStatusAuditing", model.OrderStatusAuditing, "AUDIT"},
		{"OrderStatusExecuting", model.OrderStatusExecuting, "PROCESS"},
		{"OrderStatusFailed", model.OrderStatusFailed, "ERROR"},
		{"OrderStatusWaiting", model.OrderStatusWaiting, "WAIT"},
		{"OrderStatusUndo", model.OrderStatusUndo, "Undo"},
	}
	for _, p := range pairs {
		feVal, ok := fe[p.feName]
		if !ok {
			t.Fatalf("前端 OrderState 缺少成员 %s（Go 侧 %s=%d）", p.feName, p.goName, p.goValue)
		}
		if feVal != p.goValue {
			t.Errorf("状态 %s 值漂移: Go %s=%d, 前端 %s=%d", p.feName, p.goName, p.goValue, p.feName, feVal)
		}
	}
	// 前端允许在尾部追加预留成员（如 Terminate=7），但不允许占用 Go 已定义区间的其他名字
	for name, v := range fe {
		if v >= 0 && v <= 6 {
			found := false
			for _, p := range pairs {
				if p.feName == name {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("前端 OrderState.%s=%d 落在 0..6 契约区间但无 Go 对应（两端状态机不一致）", name, v)
			}
		}
	}
}

// TestOrderTerminalStatusesMatchesEnum 终态集合必须全部落在契约状态区间内，
// 且不得包含非终态（审核/执行/等待）——归档白名单的安全底座。
func TestOrderTerminalStatusesMatchesEnum(t *testing.T) {
	allowed := map[int]bool{
		model.OrderStatusRejected: true, model.OrderStatusFinished: true,
		model.OrderStatusFailed: true, model.OrderStatusUndo: true,
	}
	for _, s := range model.OrderTerminalStatuses {
		if !allowed[s] {
			t.Errorf("OrderTerminalStatuses 含非终态 %d（会把进行中的工单归档/误杀）", s)
		}
	}
	if len(model.OrderTerminalStatuses) != 4 {
		t.Errorf("终态应为 4 个（驳回/完成/失败/撤销），实际 %v", model.OrderTerminalStatuses)
	}
}
