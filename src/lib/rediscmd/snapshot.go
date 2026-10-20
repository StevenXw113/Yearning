package rediscmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// MaxSnapshotKeys 单次抓前镜像的键数上限，超出即拒绝执行。
// 与 Mongo 侧的 MaxSnapshot 同义：不做无上限的批量变更。
const MaxSnapshotKeys = 1000

// MaxSnapshotItems 单个键的元素数上限（list/set/hash/zset）。
// 超过也拒绝执行——这种量级的回滚命令自身就是一次批量变更。
const MaxSnapshotItems = 10000

// Snapshot 一个键在变更前的状态
type Snapshot struct {
	Key    string
	Type   string // string / list / set / zset / hash / none
	Exists bool
	// TTLms 剩余过期时间（毫秒）；-1 表示不过期
	TTLms int64
	// Value 按类型的元素：string=list 单个值、list/set 为成员、
	// zset 为 member,score 交替、hash 为 field,value 交替
	Value []string
	// Unsupported 为 true 表示该类型无法快照（stream / 模块自定义类型），
	// 此时无法保证回滚，调用方应拒绝执行。
	Unsupported bool
}

// Capture 抓取前镜像：逐个键记录类型、值与剩余 TTL。
//
// 用 PTTL 而非 TTL：秒级精度会把 1.5s 的过期记成 1s，回滚后键会提早消失。
func Capture(ctx context.Context, client *redis.Client, keys []string) ([]Snapshot, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if len(keys) > MaxSnapshotKeys {
		return nil, fmt.Errorf("本次变更涉及 %d 个键，超过单次上限 %d，请分批提交", len(keys), MaxSnapshotKeys)
	}
	out := make([]Snapshot, 0, len(keys))
	for _, key := range keys {
		typ, err := client.Type(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		if typ == "none" {
			out = append(out, Snapshot{Key: key, Type: typ})
			continue
		}
		s := Snapshot{Key: key, Type: typ, Exists: true, TTLms: -1}
		if pttl, perr := client.PTTL(ctx, key).Result(); perr == nil {
			s.TTLms = pttl.Milliseconds()
		}
		if err := fillSnapshot(ctx, client, &s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// fillSnapshot 按类型读取键的值
func fillSnapshot(ctx context.Context, client *redis.Client, s *Snapshot) error {
	checkSize := func(n int64) error {
		if n > MaxSnapshotItems {
			return fmt.Errorf("键 %s 有 %d 个元素，超过快照上限 %d，请分批变更", s.Key, n, MaxSnapshotItems)
		}
		return nil
	}
	switch s.Type {
	case "string":
		v, err := client.Get(ctx, s.Key).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return err
		}
		s.Value = []string{v}
	case "list":
		n, _ := client.LLen(ctx, s.Key).Result()
		if err := checkSize(n); err != nil {
			return err
		}
		v, err := client.LRange(ctx, s.Key, 0, -1).Result()
		if err != nil {
			return err
		}
		s.Value = v
	case "set":
		n, _ := client.SCard(ctx, s.Key).Result()
		if err := checkSize(n); err != nil {
			return err
		}
		v, err := client.SMembers(ctx, s.Key).Result()
		if err != nil {
			return err
		}
		// set 无序，排序让回滚命令稳定（便于 diff 与复现）
		sort.Strings(v)
		s.Value = v
	case "zset":
		n, _ := client.ZCard(ctx, s.Key).Result()
		if err := checkSize(n); err != nil {
			return err
		}
		v, err := client.ZRangeWithScores(ctx, s.Key, 0, -1).Result()
		if err != nil {
			return err
		}
		out := make([]string, 0, len(v)*2)
		for _, z := range v {
			out = append(out, fmt.Sprint(z.Member), strconv.FormatFloat(z.Score, 'f', -1, 64))
		}
		s.Value = out
	case "hash":
		n, _ := client.HLen(ctx, s.Key).Result()
		if err := checkSize(n); err != nil {
			return err
		}
		m, err := client.HGetAll(ctx, s.Key).Result()
		if err != nil {
			return err
		}
		fields := make([]string, 0, len(m))
		for f := range m {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		out := make([]string, 0, len(fields)*2)
		for _, f := range fields {
			out = append(out, f, m[f])
		}
		s.Value = out
	default:
		// stream / 模块自定义类型：结构复杂且体积不可控，不做快照
		s.Unsupported = true
	}
	return nil
}

// Blocked 返回第一条导致「无法回滚」的原因；没有则返回 nil。
// 调用方据此在执行前拦下（与键数超限一样，属于执行底线）。
func Blocked(snaps []Snapshot) error {
	for _, s := range snaps {
		if s.Unsupported {
			return fmt.Errorf("键 %s 的类型 %s 不支持快照，无法生成回滚语句，暂不允许变更", s.Key, s.Type)
		}
	}
	return nil
}

// Rollback 把快照转成可读的回滚命令（一行一条）。
//
// 与 Mongo 侧同样的约定：回滚语句就是命令，写进 core_rollbacks，
// 重新提交即可回滚——不需要额外的反向解析逻辑。
func Rollback(snaps []Snapshot) []string {
	var out []string
	for _, s := range snaps {
		if !s.Exists {
			// 原来没有这个键：回滚就是删掉它
			out = append(out, "DEL "+Quote(s.Key))
			continue
		}
		// 先删再重建：避免目标键上残留旧结构（例如原来是 list，现在变成 string）
		out = append(out, "DEL "+Quote(s.Key))
		switch s.Type {
		case "string":
			if len(s.Value) > 0 {
				out = append(out, "SET "+Quote(s.Key)+" "+Quote(s.Value[0]))
			}
		case "list":
			if len(s.Value) > 0 {
				out = append(out, "RPUSH "+Quote(s.Key)+" "+QuoteAll(s.Value))
			}
		case "set":
			if len(s.Value) > 0 {
				out = append(out, "SADD "+Quote(s.Key)+" "+QuoteAll(s.Value))
			}
		case "zset":
			if len(s.Value) >= 2 {
				// 快照里是 member,score 交替；ZADD 要 score member 交替
				parts := make([]string, 0, len(s.Value))
				for i := 0; i+1 < len(s.Value); i += 2 {
					parts = append(parts, s.Value[i+1], s.Value[i])
				}
				out = append(out, "ZADD "+Quote(s.Key)+" "+strings.Join(parts, " "))
			}
		case "hash":
			if len(s.Value) >= 2 {
				out = append(out, "HSET "+Quote(s.Key)+" "+QuoteAll(s.Value))
			}
		}
		if s.TTLms > 0 {
			out = append(out, fmt.Sprintf("PEXPIRE %s %d", Quote(s.Key), s.TTLms))
		}
	}
	return out
}
