package rediscmd

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
)

// Outcome 一次变更执行的结果
type Outcome struct {
	// Affected 影响的键数（DEL/UNLINK 用返回值，其余按涉及键数算）
	Affected int64
	// Rollback 回滚命令：写进 core_rollbacks，重新提交即可回滚
	Rollback []string
}

// Execute 执行一条 Redis 变更命令。
//
// 顺序是有意的：先把能拦的都拦在动数据之前。
//  1. 解析 + 硬保底校验（不受规则集影响的执行底线）
//  2. 抓前镜像（此时还没动数据，抓错了也只是白读一次）
//  3. 有键无法快照（stream / 模块类型）时拒绝执行——不能保证回滚就不允许变更
//  4. 执行
//  5. 用前镜像生成回滚命令
//
// 与 Mongo 侧的差别：Redis 没有「命中行数上限」这类执行期限制需要从规则集读，
// 键数在执行前就是确定的（RedisMaxAffectKeys 已在引擎侧静态审核）。
func Execute(ctx context.Context, client *redis.Client, line string) (Outcome, error) {
	var out Outcome

	kind, args, err := Validate(line)
	if err != nil {
		return out, err
	}
	if kind == KindQuery {
		return out, errors.New("这是只读命令，不需要执行变更")
	}

	keys := AffectedKeys(args)
	snaps, err := Capture(ctx, client, keys)
	if err != nil {
		return out, err
	}
	if err := Blocked(snaps); err != nil {
		return out, err
	}

	iface := make([]interface{}, 0, len(args))
	for _, a := range args {
		iface = append(iface, a)
	}
	res, err := client.Do(ctx, iface...).Result()
	if err != nil {
		return out, err
	}

	out.Affected = affected(CommandName(args), keys, res)
	out.Rollback = Rollback(snaps)
	return out, nil
}

// affected 影响键数：DEL / UNLINK / TOUCH 的返回值就是实际命中数，
// 其余命令按涉及到的键数算（一个键改一次，记 1 是合理的口径）。
func affected(name string, keys []string, res interface{}) int64 {
	switch name {
	case "DEL", "UNLINK", "TOUCH":
		if n, ok := res.(int64); ok {
			return n
		}
	}
	return int64(len(keys))
}

// ExecuteQuery 执行一条只读命令，返回原始结果（供查询页使用）。
// 走同一套硬保底校验：查询页出现写命令在这里就被挡住。
func ExecuteQuery(ctx context.Context, client *redis.Client, line string) (interface{}, error) {
	kind, args, err := Validate(line)
	if err != nil {
		return nil, err
	}
	if kind != KindQuery {
		return nil, errors.New("查询页只能执行只读命令，变更请到工单申请页提交")
	}
	iface := make([]interface{}, 0, len(args))
	for _, a := range args {
		iface = append(iface, a)
	}
	return client.Do(ctx, iface...).Result()
}
