package mongodb

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	driver "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MaxSnapshot 单次变更允许抓取的前镜像文档上限。
// 超过就拒绝执行——说明这是一次大范围批量变更，应拆分或走人工通道；
// 否则回滚数据会撑爆元数据库，回滚本身也失去意义。
const MaxSnapshot = 1000

// Outcome 一条变更命令的执行结果
type Outcome struct {
	Affected int64    // 影响行数
	Rollback []string // 回滚命令（JSON），按顺序执行即可撤销本次变更
	Warning  string   // 无法自动回滚时的说明
}

// preState 执行前需要留存的现场
type preState struct {
	coll    string
	docs    []bson.M // update / delete 命中的文档（前镜像）
	indexes []bson.D // dropIndexes 前的索引定义
}

// Execute 执行一条变更命令，并在改之前抓取前镜像用于回滚。
// 调用方需保证命令已通过 Validate。
func Execute(ctx context.Context, client *driver.Client, db string, cmd bson.D, maxAffectRows int) (Outcome, error) {
	var out Outcome
	name := CommandName(cmd)
	kind := Classify(cmd)

	pre, err := capture(ctx, client, db, cmd, kind)
	if err != nil {
		return out, err
	}
	// 插入按提交文档数计数；更新和删除按去重后的前镜像数量计数。
	matched := len(pre.docs)
	if name == "insert" {
		if documents, ok := lookup(cmd, "documents"); ok {
			if arr, ok := toArray(documents); ok {
				matched = len(arr)
			}
		}
	}
	if maxAffectRows > 0 && matched > maxAffectRows {
		return out, fmt.Errorf("命中 %d 条文档，超过规则集上限 %d，请收窄 filter 或分批提交",
			matched, maxAffectRows)
	}

	// findAndModify returns the selected document atomically; request its pre-image for rollback.
	if name == "findandmodify" {
		fullImage := make(bson.D, 0, len(cmd)+1)
		for _, field := range cmd {
			if field.Key != "new" && field.Key != "fields" {
				fullImage = append(fullImage, field)
			}
		}
		cmd = append(fullImage, bson.E{Key: "new", Value: false})
	}

	var raw bson.M
	if err := client.Database(db).RunCommand(ctx, cmd).Decode(&raw); err != nil {
		return out, err
	}
	out.Affected = affected(name, raw)

	switch name {
	case "update":
		out.Rollback = updateRollback(pre.coll, pre.docs)
	case "delete":
		out.Rollback = deleteRollback(pre.coll, pre.docs)
	case "findandmodify":
		if before, ok := toDoc(raw["value"]); ok {
			doc := bson.M{}
			for _, field := range before {
				doc[field.Key] = field.Value
			}
			if remove, _ := lookup(cmd, "remove"); remove == true {
				out.Rollback = deleteRollback(pre.coll, []bson.M{doc})
			} else {
				out.Rollback = updateRollback(pre.coll, []bson.M{doc})
			}
		} else if info, ok := toDoc(raw["lastErrorObject"]); ok {
			if id, ok := lookup(info, "upserted"); ok {
				out.Rollback = insertRollback(pre.coll, bson.D{{Key: "documents", Value: bson.A{bson.D{{Key: "_id", Value: id}}}}})
			}
		}
	case "insert":
		out.Rollback = insertRollback(pre.coll, cmd)
	case "createindexes":
		if names := indexNames(cmd); len(names) > 0 {
			for _, n := range names {
				out.Rollback = append(out.Rollback, mustJSON(bson.D{
					{Key: "dropIndexes", Value: pre.coll}, {Key: "index", Value: n},
				}))
			}
		}
	case "dropindexes":
		if len(pre.indexes) > 0 {
			out.Rollback = []string{mustJSON(bson.D{
				{Key: "createIndexes", Value: pre.coll}, {Key: "indexes", Value: pre.indexes},
			})}
		}
	default:
		out.Warning = "该命令（" + name + "）无法自动回滚，请自行确认"
	}
	if kind != KindQuery && len(out.Rollback) == 0 && out.Warning == "" {
		out.Warning = "本次变更没有可用的前镜像，无法自动回滚"
	}
	return out, nil
}

// capture 抓取执行前现场：改数据前取命中文档，删索引前记录索引定义
func capture(ctx context.Context, client *driver.Client, db string, cmd bson.D, kind int) (preState, error) {
	pre := preState{coll: CommandCollection(cmd)}
	if kind != KindDML && CommandName(cmd) != "dropindexes" {
		return pre, nil
	}
	switch CommandName(cmd) {
	case "update", "delete":
		coll := client.Database(db).Collection(pre.coll)
		seen := map[string]struct{}{}
		for _, f := range filtersOf(cmd) {
			cur, err := coll.Find(ctx, f, options.Find().SetLimit(MaxSnapshot+1))
			if err != nil {
				return pre, err
			}
			for cur.Next(ctx) {
				var d bson.M
				if err := cur.Decode(&d); err != nil {
					_ = cur.Close(ctx)
					return pre, err
				}
				id := cur.Current.Lookup("_id")
				key := string([]byte{byte(id.Type)}) + string(id.Value)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				pre.docs = append(pre.docs, d)
				if len(pre.docs) > MaxSnapshot {
					_ = cur.Close(ctx)
					return pre, fmt.Errorf("命中文档超过 %d 条，请收窄 filter 或分批提交工单", MaxSnapshot)
				}
			}
			if err := cur.Err(); err != nil {
				_ = cur.Close(ctx)
				return pre, err
			}
			_ = cur.Close(ctx)
		}
	case "dropindexes":
		coll := client.Database(db).Collection(pre.coll)
		// 删掉的索引定义要留下来，否则回滚时无法重建
		cur, err := coll.Indexes().List(ctx)
		if err != nil {
			return pre, err
		}
		for cur.Next(ctx) {
			var idx bson.D
			if err := cur.Decode(&idx); err != nil {
				_ = cur.Close(ctx)
				return pre, err
			}
			if v, ok := lookup(idx, "name"); ok && v == "_id_" {
				continue // _id 索引删不掉，也不该出现在重建语句里
			}
			pre.indexes = append(pre.indexes, idx)
		}
		_ = cur.Close(ctx)
	}
	return pre, nil
}

// updateRollback 用前镜像把文档整体改回去（replace 语义 + upsert 兜底：
// 万一原文档在变更中被删了，回滚也能把它放回来）
func updateRollback(coll string, before []bson.M) []string {
	if len(before) == 0 {
		return nil
	}
	updates := make(bson.A, 0, len(before))
	for _, d := range before {
		updates = append(updates, bson.D{
			{Key: "q", Value: bson.D{{Key: "_id", Value: d["_id"]}}},
			{Key: "u", Value: toDocValue(d)},
			{Key: "upsert", Value: true},
		})
	}
	return []string{mustJSON(bson.D{{Key: "update", Value: coll}, {Key: "updates", Value: updates}})}
}

// deleteRollback 把删掉的文档原样插回去
func deleteRollback(coll string, before []bson.M) []string {
	if len(before) == 0 {
		return nil
	}
	docs := make(bson.A, 0, len(before))
	for _, d := range before {
		docs = append(docs, toDocValue(d))
	}
	return []string{mustJSON(bson.D{{Key: "insert", Value: coll}, {Key: "documents", Value: docs}})}
}

// insertRollback 按 _id 删掉本次插入的文档。
// _id 只能从提交的命令里取：insert 命令的响应只回 {"n":1}，
// 那个带 insertedIds 的形态是驱动 InsertOne 的封装，runCommand 拿不到。
// 文档不带 _id 时由服务端生成，此时无从回滚（调用方会给出 Warning）。
func insertRollback(coll string, cmd bson.D) []string {
	ids := documentIDs(cmd)
	if len(ids) == 0 {
		return nil
	}
	return []string{mustJSON(bson.D{
		{Key: "delete", Value: coll},
		{Key: "deletes", Value: bson.A{bson.D{
			{Key: "q", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}}},
			{Key: "limit", Value: 0},
		}}},
	})}
}

// documentIDs 取 insert 命令里各文档的 _id
func documentIDs(cmd bson.D) []interface{} {
	v, ok := lookup(cmd, "documents")
	if !ok {
		return nil
	}
	arr, ok := toArray(v)
	if !ok {
		return nil
	}
	var ids []interface{}
	for _, item := range arr {
		d, ok := toDoc(item)
		if !ok {
			continue
		}
		if id, ok := lookup(d, "_id"); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// indexNames 取 createIndexes 里声明的索引名
func indexNames(cmd bson.D) []string {
	v, ok := lookup(cmd, "indexes")
	if !ok {
		return nil
	}
	arr, ok := toArray(v)
	if !ok {
		return nil
	}
	var names []string
	for _, item := range arr {
		idx, ok := toDoc(item)
		if !ok {
			continue
		}
		if n, ok := lookup(idx, "name"); ok {
			if s, ok := n.(string); ok {
				names = append(names, s)
			}
		}
	}
	return names
}

// affected 从命令响应里取影响行数。
// 不同命令/版本的 n 形态不一：数字（{n:3}）或文档（{n:{inserted:2}}），两种都认。
func affected(name string, raw bson.M) int64 {
	if v, ok := raw["n"]; ok {
		if n, ok := toInt64(v); ok {
			return n
		}
		if d, ok := toDoc(v); ok {
			var n int64
			for _, e := range d {
				if n2, ok := toInt64(e.Value); ok {
					n += n2
				}
			}
			return n
		}
	}
	if name == "findandmodify" {
		if info, ok := toDoc(raw["lastErrorObject"]); ok {
			if n, ok := lookup(info, "n"); ok {
				if count, ok := toInt64(n); ok {
					return count
				}
			}
		}
		if raw["value"] != nil {
			return 1
		}
	}
	return 0
}

func toInt64(v interface{}) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	}
	return 0, false
}

// toDocValue 把前端/驱动给出的 map 表示转成 bson.D（写回时保持可读的顺序）
func toDocValue(v interface{}) bson.D {
	if d, ok := toDoc(v); ok {
		return d
	}
	return bson.D{}
}

// mustJSON 把 bson 值编成可读 JSON（relaxed 扩展 JSON），作为回滚语句存库
func mustJSON(v interface{}) string {
	b, err := bson.MarshalExtJSON(v, false, false)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
