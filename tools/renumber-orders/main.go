// Command renumber-orders 把历史工单编号收敛成「自增 id」形态。
//
// 规则：
//   - 普通 SQL 工单 / 查询工单：work_id = 自身自增 id（如 144）
//   - 项目级工单：项目号 = 项目内首条子工单的自增 id，子工单 work_id = 项目号-序号
//     （如 150-1、150-2），同时把该行 batch_id 也改成这个项目号
//
// 幂等，可重复执行：目标编号只由 id / batch_id 推出，与当前值相同即跳过。
// 会一并改写引用旧编号的其它表（这些表只能按 work_id 定位）：
//   - SQL 工单：core_sql_records / core_rollbacks / core_workflow_details / core_order_comments
//   - 查询工单：core_query_records
//
// 默认只预演，加 -apply 才写入：
//
//	go run ./tools/renumber-orders -meta-dsn 'root:pwd@tcp(<元数据库>:3306)/Yearning_go?charset=utf8mb4'
//	go run ./tools/renumber-orders -meta-dsn '...' -apply
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
)

var (
	metaDSN = flag.String("meta-dsn", "", "元数据库 DSN（必填）")
	apply   = flag.Bool("apply", false, "真正写入；不加则只预演打印")
)

// job 一张工单主表，以及引用它编号、只能按 work_id 定位的其它表
type job struct {
	table    string
	hasBatch bool // 主表是否有 batch_id（项目号）需要一起收敛
	refs     []string
}

func jobs() []job {
	return []job{
		{"core_sql_orders", true, []string{"core_sql_records", "core_rollbacks", "core_workflow_details", "core_order_comments"}},
		{"core_query_orders", false, []string{"core_query_records"}},
	}
}

type rename struct {
	id       uint64
	old      string
	new      string
	newBatch string // 项目级工单的新 batch_id（项目号），非项目级为空
}

func fatal(v any) {
	fmt.Fprintln(os.Stderr, "renumber-orders:", v)
	os.Exit(1)
}

// plan 按自增 id 算出每张主表里需要改编号的行
func plan(db *sql.DB, j job) ([]rename, error) {
	query := "SELECT id, work_id FROM " + j.table + " ORDER BY id"
	if j.hasBatch {
		query = "SELECT id, work_id, batch_id FROM " + j.table + " ORDER BY id"
	}
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []rename
	firstID := map[string]uint64{} // 批次 -> 项目内首条子工单 id
	seq := map[string]int{}        // 批次 -> 已分配序号
	for rows.Next() {
		var id uint64
		var wid, batch string
		if j.hasBatch {
			err = rows.Scan(&id, &wid, &batch)
		} else {
			err = rows.Scan(&id, &wid)
		}
		if err != nil {
			return nil, err
		}
		if batch == "" {
			if nw := strconv.FormatUint(id, 10); nw != wid {
				out = append(out, rename{id: id, old: wid, new: nw})
			}
			continue
		}
		if _, ok := firstID[batch]; !ok {
			firstID[batch] = id
		}
		seq[batch]++
		projectNo := strconv.FormatUint(firstID[batch], 10)
		nw := fmt.Sprintf("%s-%d", projectNo, seq[batch])
		if nw != wid || batch != projectNo {
			out = append(out, rename{id: id, old: wid, new: nw, newBatch: projectNo})
		}
	}
	return out, rows.Err()
}

func main() {
	flag.Parse()
	if *metaDSN == "" {
		flag.Usage()
		os.Exit(2)
	}
	db, err := sql.Open("mysql", *metaDSN)
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	js := jobs()
	plans := make([][]rename, len(js))
	oldIDs := map[string]bool{}
	newIDs := map[string]bool{}
	total := 0
	for i, j := range js {
		p, err := plan(db, j)
		if err != nil {
			fatal(err)
		}
		plans[i] = p
		total += len(p)
		for _, r := range p {
			oldIDs[r.old] = true
			newIDs[r.new] = true
		}
	}
	// 护栏：新编号若与某个待改的旧编号相同，会出现 A->B、B->C 的链式覆盖（后一步会改掉前一步的结果）
	for n := range newIDs {
		if oldIDs[n] {
			fatal(fmt.Errorf("新编号 %q 与待改旧编号撞车，请先人工处理后重跑", n))
		}
	}

	if total == 0 {
		fmt.Println("没有需要重新编号的工单（都已是自增 id 形态）。")
		return
	}

	for i, j := range js {
		if len(plans[i]) == 0 {
			continue
		}
		refRows := 0
		for _, r := range plans[i] {
			for _, t := range j.refs {
				var c int
				_ = db.QueryRow("SELECT count(*) FROM "+t+" WHERE work_id = ?", r.old).Scan(&c)
				refRows += c
			}
		}
		fmt.Printf("%s：待改 %d 条（引用行 %d）\n", j.table, len(plans[i]), refRows)
		for k, r := range plans[i] {
			if k >= 5 {
				fmt.Printf("  ... 其余 %d 条从略\n", len(plans[i])-5)
				break
			}
			fmt.Printf("  id=%-6d %s -> %s\n", r.id, r.old, r.new)
		}
	}

	if !*apply {
		fmt.Printf("\n以上 %d 条为预演；确认无误后加 -apply 执行。\n", total)
		return
	}

	tx, err := db.Begin()
	if err != nil {
		fatal(err)
	}
	for i, j := range js {
		for _, r := range plans[i] {
			var err error
			if j.hasBatch {
				_, err = tx.Exec("UPDATE "+j.table+" SET work_id = ?, batch_id = ? WHERE id = ?", r.new, r.newBatch, r.id)
			} else {
				_, err = tx.Exec("UPDATE "+j.table+" SET work_id = ? WHERE id = ?", r.new, r.id)
			}
			if err != nil {
				_ = tx.Rollback()
				fatal(err)
			}
			for _, t := range j.refs {
				if _, err := tx.Exec("UPDATE "+t+" SET work_id = ? WHERE work_id = ?", r.new, r.old); err != nil {
					_ = tx.Rollback()
					fatal(err)
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		fatal(err)
	}
	fmt.Printf("\n已完成 %d 条工单重新编号，请重启主程序后核对。\n", total)
}
