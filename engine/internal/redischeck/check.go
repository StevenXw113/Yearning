// Package redischeck 提供 Redis 命令的静态审核：bytebase 无 Redis advisor，
// 这里按 RESP 参数规则切词、用「命令名 + 参数形态」判定；只用标准库，不引 go-redis。
package redischeck

import "errors"

// Check 审核一条命令；第三个返回值非 nil 表示命令本身不可用。
func Check(line string, cfg Config) (int, []Finding, error) {
	cmd, err := Parse(line)
	if err != nil {
		return KindQuery, nil, err
	}

	if cfg.queryMode() {
		// 查询页出现写命令直接拒：能改数据等于绕过审批，不该做成开关。
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

func notReadOnly(name string) string {
	return "查询页只能执行只读命令（GET / SCAN / HGETALL / LRANGE 等），" +
		name + " 属于变更命令，请到工单申请页提交"
}
