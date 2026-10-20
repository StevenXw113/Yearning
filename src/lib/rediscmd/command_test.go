package rediscmd

import (
	"strings"
	"testing"
)

func TestSplit(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`SET key value`, []string{"SET", "key", "value"}},
		{`SET "key with space" v`, []string{"SET", "key with space", "v"}},
		{`SET k "a\nb"`, []string{"SET", "k", "a\nb"}},
		{`SET k 'a\'b'`, []string{"SET", "k", "a'b"}},
		{`  SET   k   v  `, []string{"SET", "k", "v"}},
		{`SET k ""`, []string{"SET", "k", ""}},
		{`HSET h f "\x41\x42"`, []string{"HSET", "h", "f", "AB"}},
		{`EXPIRE k 60`, []string{"EXPIRE", "k", "60"}},
	}
	for _, c := range cases {
		got, err := Split(c.in)
		if err != nil {
			t.Errorf("Split(%q) 报错: %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("Split(%q) = %q, want %q", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("Split(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
	if _, err := Split(`SET "unclosed`); err == nil {
		t.Error("未闭合引号应报错")
	}
}

func TestValidate(t *testing.T) {
	allow := []string{
		"GET yrn:test:user:1",
		"SCAN 0 MATCH yrn:test:* COUNT 100",
		"HSET yrn:test:profile:1 name alice",
		"DEL yrn:test:user:2",
		"SET yrn:test:k v EX 60",
		"LPUSH yrn:test:queue a b",
	}
	for _, line := range allow {
		if _, _, err := Validate(line); err != nil {
			t.Errorf("Validate(%q) 应通过, got %v", line, err)
		}
	}

	deny := map[string]string{
		"FLUSHALL":                       "FLUSHALL",
		"FLUSHDB":                        "FLUSHDB",
		"SHUTDOWN NOSAVE":                "SHUTDOWN",
		"CONFIG SET maxmemory 0":         "CONFIG",
		"DEBUG SLEEP 10":                 "DEBUG",
		`EVAL "return 1" 0`:              "EVAL",
		`SCRIPT LOAD "return 1"`:         "SCRIPT",
		"REPLICAOF NO ONE":               "REPLICAOF",
		"MONITOR":                        "MONITOR",
		"KEYS *":                         "KEYS",
		"MIGRATE 10.0.0.1 6379 k 0 1000": "MIGRATE",
	}
	for line, want := range deny {
		_, _, err := Validate(line)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Validate(%q) 应报含 %q 的错误, got %v", line, want, err)
		}
	}

	if _, _, err := Validate("   "); err == nil {
		t.Error("空命令应报错")
	}
}

func TestValidateReturnsArgs(t *testing.T) {
	// 返回的 args 应可直接交给驱动执行（引号已剥掉）
	_, args, err := Validate(`SET "yrn:test:key with space" "value"`)
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 3 || args[1] != "yrn:test:key with space" || args[2] != "value" {
		t.Errorf("args = %q, want 去掉引号后的三段", args)
	}
}

func TestClassify(t *testing.T) {
	if k, _, _ := Validate("GET k"); k != KindQuery {
		t.Error("GET 应为查询")
	}
	if k, _, _ := Validate("SET k v"); k != KindDML {
		t.Error("SET 应为 DML")
	}
	if k, _, _ := Validate("DEL k"); k != KindDML {
		t.Error("DEL 应为 DML")
	}
	if k, _, _ := Validate("HSET h f v"); k != KindDML {
		t.Error("HSET 应为 DML")
	}
	// 未知命令按变更处理：宁可走审批，也别把写命令当查询放行
	if k, _, _ := Validate("SOMENEWCMD x"); k != KindDDL {
		t.Error("未知命令应按变更处理")
	}
}

func TestCommandNameCaseInsensitive(t *testing.T) {
	if n := CommandName([]string{"get", "k"}); n != "GET" {
		t.Errorf("命令名应大写归一, got %q", n)
	}
	// 小写命令同样要能被危险命令表拦下
	if _, _, err := Validate("flushall"); err == nil {
		t.Error("小写 flushall 也应被拦")
	}
	if _, _, err := Validate("keys *"); err == nil {
		t.Error("小写 keys 也应被拦")
	}
}
