package mongodb

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	driver "go.mongodb.org/mongo-driver/mongo"
)

// indexWatchInterval 进度轮询间隔
const indexWatchInterval = 2 * time.Second

// indexProgress 一条进度：Key 用于去重（同一百分比只报一次），Line 是给面板显示的文本
type indexProgress struct {
	Key  string
	Line string
}

// WatchIndexBuild 轮询索引构建进度，每出现一个新的百分比就回调一行文本；
// 返回的 stop 用于结束轮询。
//
// 为什么需要它：Mongo 4.2 起建索引是混合锁（首尾短暂排他、中间并发），大集合上
// 依旧可能跑很久，但它是后台操作、不会像 gh-ost 那样自己打印进度。这里用
// `currentOp` 把进度取出来写进 core_sql_orders.osc_info，复用详情页现成的
// 「在线变更」面板（该面板只做整段文本渲染，前端零改动）。
//
// 看不到进度不该影响变更本身：权限不足（currentOp 需要 inprog / clusterMonitor）
// 或实例不支持时静默跳过。
func WatchIndexBuild(ctx context.Context, client *driver.Client, database, collection string, emit func(string)) (stop func()) {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(indexWatchInterval)
		defer ticker.Stop()
		reported := map[string]struct{}{}
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				items, err := indexBuildProgress(ctx, client, database, collection)
				if err != nil {
					continue
				}
				for _, it := range items {
					if _, ok := reported[it.Key]; ok {
						continue
					}
					reported[it.Key] = struct{}{}
					emit(it.Line)
				}
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

// indexBuildProgress 取本库（可选本集合）正在进行的索引构建进度。
// 按「集合 + 整数百分比」去重：一次构建最多产出百余条，不会把 osc_info 写爆。
func indexBuildProgress(ctx context.Context, client *driver.Client, database, collection string) ([]indexProgress, error) {
	var res struct {
		Inprog []bson.M `bson:"inprog"`
	}
	if err := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "currentOp", Value: 1},
		{Key: "$ownOps", Value: true},
	}).Decode(&res); err != nil {
		return nil, err
	}

	var out []indexProgress
	for _, op := range res.Inprog {
		msg, _ := op["msg"].(string)
		if !strings.HasPrefix(msg, "Index Build") {
			continue
		}
		target := ""
		if raw, ok := op["command"]; ok {
			if doc, ok := toDoc(raw); ok {
				target = CommandCollection(doc)
			}
		}
		if collection != "" && target != collection {
			continue
		}
		progress, ok := asMapOf(op["progress"])
		if !ok {
			continue
		}
		done, _ := toInt64(progress["done"])
		total, _ := toInt64(progress["total"])
		pct := int64(0)
		if total > 0 {
			pct = done * 100 / total
		}
		name := target
		if name == "" {
			name = "集合"
		}
		out = append(out, indexProgress{
			// 去重键用 done 而不是百分比：Mongo 建索引分多趟扫描，计数会重置，
			// 同一百分比可能出现多次（例如 37% → 98% → 31%），按百分比去重会漏报
			Key:  fmt.Sprintf("%s/%d", target, done),
			Line: fmt.Sprintf("[%s] 索引构建 %s: %d/%d %d%%", time.Now().Format("15:04:05"), name, done, total, pct),
		})
	}
	return out, nil
}

// asMapOf 把驱动给出的几种文档表示统一成 map（只读场景，顺序无所谓）
func asMapOf(v interface{}) (map[string]interface{}, bool) {
	switch x := v.(type) {
	case bson.M:
		return x, true
	case map[string]interface{}:
		return x, true
	case bson.D:
		return x.Map(), true
	}
	return nil, false
}
