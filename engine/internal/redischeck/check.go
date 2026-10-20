// Package redischeck 实现 Redis 命令的静态审核。
//
// 上游 bytebase 不提供 Redis 审核规则：common.EngineSupportSQLReview(REDIS)
// 返回 false，advisor 目录下也没有 redis 方言——proto 里的 Engine_REDIS 只是
// 随包内化带进来的枚举值（能力矩阵必须穷举它），engine 的解析/审核链路一处都没用到。
// 所以这一层是自研的。
//
// 与 SQL 侧的根本差别：SQL 审核建立在语法树之上，而 Redis 命令是一行文本，
// 没有 AST 可依附。这里退一步，按 RESP 参数规则切词，再用「命令名 + 参数形态」
// 做约束——够挡住最常见也最危险的操作（清库、服务端脚本、阻塞式全键扫描、
// 运维管理命令、无 TTL 的写入）。
//
// 结构：规则注册表（rule.go）+ 共享解析层（command.go）+ 规则开关（config.go），
// 一条规则一个文件、在 init() 里注册。
//
// 只用标准库解析：本包不引入 go-redis，避免引擎依赖变重。
package redischeck

import "errors"

// Check 审核一条 Redis 命令。
//
// 返回变更类型与命中的规则；第三个返回值非 nil 表示命令本身不可用：
// 空命令、引号未闭合，或查询模式下出现的写命令。
func Check(line string, cfg Config) (int, []Finding, error) {
	cmd, err := Parse(line)
	if err != nil {
		return KindQuery, nil, err
	}

	if cfg.queryMode() {
		// 查询页出现写命令：直接拒绝。这不是可配置的规则——
		// 查询页能改数据等于绕过整个工单审批链路，不该依赖谁把开关配对。
		if cmd.Kind != KindQuery {
			return cmd.Kind, nil, errors.New(notReadOnly(cmd.Name))
		}
	} else if cmd.Kind == KindQuery {
		return cmd.Kind, nil, errors.New("不是变更命令（SET / DEL / HSET / LPUSH 等），无需提交工单")
	}

	var out []Finding
	for _, r := range All() {
		if !applies(r, cfg.mode()) {
			continue
		}
		out = append(out, r.Check(cfg, cmd)...)
	}
	return cmd.Kind, out, nil
}

// notReadOnly 查询页的拒绝文案：说清为什么不行、以及该去哪儿做
func notReadOnly(name string) string {
	return "查询页只能执行只读命令（GET / SCAN / HGETALL / LRANGE 等），" +
		name + " 属于变更命令，请到工单申请页提交"
}
