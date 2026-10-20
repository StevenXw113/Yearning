package rediscmd

import "strings"

// keyAllArgs 这些命令的全部参数都是键
var keyAllArgs = map[string]struct{}{
	"DEL": {}, "UNLINK": {}, "TOUCH": {}, "MGET": {}, "PFMERGE": {}, "WATCH": {},
}

// keyFrom2 这些命令从第 2 个参数起都是键（第 1 个是操作符）
// 例：BITOP AND dest src1 src2
var keyFrom2 = map[string]struct{}{
	"BITOP": {},
}

// keyPair 前两个参数都是键：源与目标（目标会被覆盖，必须一起抓）
var keyPair = map[string]struct{}{
	"RENAME": {}, "RENAMENX": {}, "COPY": {},
	"SMOVE": {}, "LMOVE": {}, "BLMOVE": {}, "RPOPLPUSH": {}, "BRPOPLPUSH": {},
}

// AffectedKeys 返回一条命令**可能改动**的键，用于抓前镜像。
//
// 只关心写路径：纯读命令返回空。刻意不做完整性妥协——
// 少抓一个键就意味着回滚时那个键回不去，所以宁可多抓（例如 RENAME 的源与目标都抓）。
func AffectedKeys(args []string) []string {
	if len(args) < 2 {
		return nil
	}
	name := CommandName(args)

	switch name {
	case "MSET", "MSETNX":
		// 键值成对：1,3,5…
		var out []string
		for i := 1; i < len(args); i += 2 {
			out = append(out, args[i])
		}
		return dedup(out)
	case "GETDEL", "GETEX", "GETSET":
		// 读的同时会删键（GETEX 去掉 TTL 也算改动）
		return dedup(args[1:2])
	}

	if _, ok := keyAllArgs[name]; ok {
		return dedup(args[1:])
	}
	if _, ok := keyFrom2[name]; ok {
		return dedup(args[2:])
	}
	if _, ok := keyPair[name]; ok {
		if len(args) > 2 {
			return dedup(args[1:3])
		}
		return dedup(args[1:])
	}
	// 其余写命令的约定：第一个参数是键
	if isWriteCommand(name) {
		return dedup(args[1:2])
	}
	return nil
}

// dedup 去重且保持出现顺序（去重后回滚命令里不会重复处理同一个键）
func dedup(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Quote 把命令参数按 redis-cli 的规则加引号。
//
// 它与 Split 是互逆的：Rollback 生成的回滚命令必须能被 Split 原样解析回来，
// 否则键名里有空格/引号时回滚就会打偏。只在必要时加引号，保持可读。
func Quote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\r\n\"'\\") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteByte(s[i])
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}

// QuoteAll 批量加引号并用空格连接
func QuoteAll(items []string) string {
	parts := make([]string, 0, len(items))
	for _, s := range items {
		parts = append(parts, Quote(s))
	}
	return strings.Join(parts, " ")
}

// CommandText 把参数拼回一行命令（用于把命令写进工单明细）
func CommandText(args []string) string {
	return QuoteAll(args)
}
