// Command shorten-source-id 把历史遗留的 UUID 形态数据源 ID 收敛成 4 位短 ID。
//
// 幂等、可重复执行；已建立的数据源 ID 若已经是短的会直接跳过。
//
// 会一并改写所有引用它的地方：
//
//   - core_sql_orders / core_query_orders / core_auto_tasks 的 source_id
//   - core_role_groups.permissions 里 ddl_source / dml_source / query_source 列表中的旧 ID
//     （权限是按 source_id 授权的，漏改会让这些数据源对所有人变成「没有权限」）
//
// 默认只打印将要做的改动（预演），加 -apply 才真正写入：
//
//	go run ./tools/shorten-source-id -meta-dsn 'root:pwd@tcp(<元数据库>:3306)/Yearning_go?charset=utf8mb4'
//	go run ./tools/shorten-source-id -meta-dsn '...' -apply
package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 去掉了容易看混的字符（0/O、1/l/I）
const alphabet = "23456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ"

var (
	metaDSN = flag.String("meta-dsn", "", "元数据库 DSN（必填）")
	apply   = flag.Bool("apply", false, "真正写入；不加则只预演打印")
	idLen   = flag.Int("len", 4, "数据源 ID 目标长度")
)

// 引用 source_id 的业务表
var refTables = []string{"core_sql_orders", "core_query_orders", "core_auto_tasks"}

// permissionKeys 是权限 JSON 里按数据源授权的三个列表
var permissionKeys = []string{"ddl_source", "dml_source", "query_source"}

func genID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	for i, v := range b {
		b[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(b)
}

func isUUID(s string) bool {
	return len(s) == 36 && strings.Count(s, "-") == 4
}

func main() {
	flag.Parse()
	if *metaDSN == "" {
		flag.Usage()
		os.Exit(2)
	}
	db, err := sql.Open("mysql", *metaDSN)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shorten-source-id:", err)
		os.Exit(2)
	}
	defer db.Close()

	type source struct {
		id     int
		name   string
		source string
	}
	var sources []source
	rows, err := db.Query("SELECT id, source, source_id FROM core_data_sources")
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取数据源失败:", err)
		os.Exit(2)
	}
	used := map[string]bool{}
	for rows.Next() {
		var s source
		var sid string
		if err := rows.Scan(&s.id, &s.name, &sid); err != nil {
			continue
		}
		s.source = sid
		if !isUUID(sid) {
			used[sid] = true
		}
		sources = append(sources, s)
	}
	rows.Close()

	changed := 0
	for _, s := range sources {
		if !isUUID(s.source) {
			continue
		}
		newID := ""
		for n := *idLen; n <= 8 && newID == ""; n++ {
			for i := 0; i < 32; i++ {
				cand := genID(n)
				if !used[cand] {
					newID = cand
					break
				}
			}
		}
		if newID == "" {
			fmt.Printf("跳过 %s：短 ID 空间分配失败，请加长 -len\n", s.name)
			continue
		}
		used[newID] = true

		refs := map[string]int{}
		total := 0
		for _, t := range refTables {
			var c int
			_ = db.QueryRow("SELECT count(*) FROM "+t+" WHERE source_id = ?", s.source).Scan(&c)
			refs[t] = c
			total += c
		}
		permRows := 0
		var groups []struct {
			id   int
			name string
			perm string
		}
		grows, err := db.Query("SELECT id, name, permissions FROM core_role_groups")
		if err == nil {
			for grows.Next() {
				var g struct {
					id   int
					name string
					perm string
				}
				if err := grows.Scan(&g.id, &g.name, &g.perm); err == nil {
					if strings.Contains(g.perm, s.source) {
						permRows++
					}
					groups = append(groups, g)
				}
			}
			grows.Close()
		}

		fmt.Printf("数据源 %-12s %s -> %s（引用：工单 %d / 查询工单 %d / 自动任务 %d；权限组 %d 处）\n",
			s.name, s.source, newID, refs["core_sql_orders"], refs["core_query_orders"], refs["core_auto_tasks"], permRows)
		changed++

		if !*apply {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			fmt.Fprintln(os.Stderr, "开启事务失败:", err)
			os.Exit(2)
		}
		ok := true
		fail := func(what string, err error) {
			fmt.Fprintf(os.Stderr, "%s 失败，整体回滚: %v\n", what, err)
			_ = tx.Rollback()
			ok = false
		}
		if _, err := tx.Exec("UPDATE core_data_sources SET source_id = ? WHERE id = ?", newID, s.id); err != nil {
			fail("更新数据源", err)
		}
		if ok {
			for _, t := range refTables {
				if _, err := tx.Exec("UPDATE "+t+" SET source_id = ? WHERE source_id = ?", newID, s.source); err != nil {
					fail("更新 "+t, err)
					break
				}
			}
		}
		if ok {
			for _, g := range groups {
				np, changedPerm := rewritePermission(g.perm, s.source, newID)
				if !changedPerm {
					continue
				}
				if _, err := tx.Exec("UPDATE core_role_groups SET permissions = ? WHERE id = ?", np, g.id); err != nil {
					fail("更新权限组 "+g.name, err)
					break
				}
			}
		}
		if !ok {
			os.Exit(1)
		}
		if err := tx.Commit(); err != nil {
			fmt.Fprintln(os.Stderr, "提交失败:", err)
			os.Exit(1)
		}
	}

	fmt.Println()
	if changed == 0 {
		fmt.Println("没有需要处理的数据源（都已是短 ID）。")
		return
	}
	if *apply {
		fmt.Printf("已完成 %d 个数据源的 ID 收敛，请重启主程序后核对权限与数据源列表。\n", changed)
		return
	}
	fmt.Printf("以上 %d 个数据源待收敛；确认无误后加 -apply 执行。\n", changed)
}

// rewritePermission 把权限 JSON 里 *_source 列表中的 oldID 换成 newID。
func rewritePermission(perm, oldID, newID string) (string, bool) {
	if perm == "" {
		return perm, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(perm), &m); err != nil {
		return perm, false
	}
	changed := false
	for _, k := range permissionKeys {
		list, ok := m[k].([]any)
		if !ok {
			continue
		}
		for i, v := range list {
			if s, ok := v.(string); ok && s == oldID {
				list[i] = newID
				changed = true
			}
		}
		m[k] = list
	}
	if !changed {
		return perm, false
	}
	out, err := json.Marshal(m)
	if err != nil {
		return perm, false
	}
	return string(out), true
}
