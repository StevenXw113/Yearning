package server

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bytebase/omni/mysql/ast"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"

	enginev1 "engine/gen/engine/v1"
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/parser/base"
	mysqlparser "engine/internal/bytebase/plugin/parser/mysql"
)

// binlogCaptureTimeout 是等待 binlog 事件的上限：超过就用手头已有的事件生成回滚，不阻塞执行。
const binlogCaptureTimeout = 10 * time.Second

// binlogCapture 用复制协议抓取工单真正写入的行事件，据此生成精确回滚语句。
//
// 相比「执行前 SELECT 前镜像」，它拿的是引擎落盘的真实 before/after image，因此覆盖全部 DML 场景：
// 多表 UPDATE/DELETE、无 WHERE 的全表操作、无主键表、INSERT...SELECT、REPLACE 等。
// 前提是源库开启 binlog（ROW + row_image=FULL）且数据源账号有 REPLICATION SLAVE/CLIENT 权限；
// 不满足时由调用方退回到前镜像方案。
type binlogCapture struct {
	cfg    replication.BinlogSyncerConfig
	meta   *tableMeta
	tables map[string]bool // 只收集工单目标表的事件（窗口内的其他写入不属于本工单）
	start  mysql.Position
}

// newBinlogCapture 记录工单执行前的 binlog 位点；binlog 不可用时返回 false，调用方改用前镜像方案。
func newBinlogCapture(ctx context.Context, src *enginev1.DataSource, db *sql.DB, meta *tableMeta, schema string, stmts []string) (*binlogCapture, bool) {
	if src == nil || src.Ip == "" {
		return nil, false
	}
	start, ok := masterStatus(ctx, db)
	if !ok {
		return nil, false
	}
	// 只保留真实存在的表：DELETE u FROM a4 u 这类写法会把别名 u 也解析成表引用，
	// 用存在性校验剔掉别名，同时保证不会漏掉真正的目标表。
	tables := map[string]bool{}
	for name := range targetTables(schema, stmts) {
		db, table, ok := strings.Cut(name, ".")
		if ok && len(meta.columns(ctx, db, table)) > 0 {
			tables[name] = true
		}
	}
	if len(tables) == 0 {
		return nil, false
	}
	return &binlogCapture{
		cfg: replication.BinlogSyncerConfig{
			// server_id 需与源库及其他从库不同，取时间戳低位足够避免碰撞
			ServerID: uint32(100000 + time.Now().UnixNano()%1000000),
			Flavor:   "mysql",
			Host:     src.Ip,
			Port:     uint16(src.Port),
			User:     src.Username,
			Password: src.Password,
			Charset:  "utf8mb4",
		},
		meta:   meta,
		tables: tables,
		start:  start,
	}, true
}

// masterStatus 读取当前 binlog 位点。未开 binlog 或无 REPLICATION CLIENT 权限时返回 false。
func masterStatus(ctx context.Context, db *sql.DB) (mysql.Position, bool) {
	for _, q := range []string{"SHOW BINARY LOG STATUS", "SHOW MASTER STATUS"} {
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			continue
		}
		cols, err := rows.Columns()
		if err != nil || len(cols) < 2 {
			rows.Close()
			continue
		}
		var file, pos string
		found := false
		for rows.Next() {
			vals := make([]sql.NullString, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				break
			}
			file, pos = vals[0].String, vals[1].String
			found = true
		}
		rows.Close()
		if !found {
			continue
		}
		p, err := strconv.ParseUint(pos, 10, 32)
		if err != nil {
			continue
		}
		return mysql.Position{Name: file, Pos: uint32(p)}, true
	}
	return mysql.Position{}, false
}

// rollback 抓取 [start, end) 区间的行事件，生成按事件逆序排列的回滚语句。
// end 由调用方在执行完成（含提交）后读取，保证覆盖工单的全部改动。
func (c *binlogCapture) rollback(ctx context.Context, end mysql.Position) []string {
	if c.start.Name == "" || end.Name == "" {
		return nil
	}
	syncer := replication.NewBinlogSyncer(c.cfg)
	defer syncer.Close()
	streamer, err := syncer.StartSync(c.start)
	if err != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, binlogCaptureTimeout)
	defer cancel()

	cur := c.start.Name
	var stmts []string
	for {
		ev, err := streamer.GetEvent(ctx)
		if err != nil {
			break // 超时或连接中断：用手头已有的事件
		}
		switch e := ev.Event.(type) {
		case *replication.RotateEvent:
			cur = string(e.NextLogName)
		case *replication.RowsEvent:
			stmts = append(stmts, c.rowsRollback(ctx, ev.Header.EventType, e)...)
		}
		if cur > end.Name || (cur == end.Name && ev.Header.LogPos >= end.Pos) {
			break
		}
	}
	// 逆序执行才能正确撤销：最后一条改动最先还原。
	for i, j := 0, len(stmts)-1; i < j; i, j = i+1, j-1 {
		stmts[i], stmts[j] = stmts[j], stmts[i]
	}
	return stmts
}

// rowsRollback 把一条行事件转成回滚语句。
func (c *binlogCapture) rowsRollback(ctx context.Context, t replication.EventType, e *replication.RowsEvent) []string {
	if e.Table == nil {
		return nil
	}
	db, table := string(e.Table.Schema), string(e.Table.Table)
	if !c.tables[db+"."+table] {
		return nil // 窗口内其他会话/其他表的写入不属于本工单
	}
	cols := c.meta.columns(ctx, db, table)
	if len(cols) == 0 {
		return nil
	}
	pks := c.meta.primaryKeys(ctx, db, table)

	var out []string
	switch {
	case isWriteRows(t): // 插入的行：镜像即插入后的样子
		for _, row := range e.Rows {
			if s := deleteByImage(db, table, cols, row, pks); s != "" {
				out = append(out, s)
			}
		}
	case isDeleteRows(t): // 删除的行：镜像即删除前的内容，塞回去即可
		for _, row := range e.Rows {
			if s := insertByImage(db, table, cols, row); s != "" {
				out = append(out, s)
			}
		}
	case isUpdateRows(t): // 镜像成对出现：before, after, before, after...
		for i := 0; i+1 < len(e.Rows); i += 2 {
			if s := updateByImage(db, table, cols, e.Rows[i], e.Rows[i+1], pks); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// deleteByImage 生成「撤销 INSERT」的语句：按行镜像删掉这一行。
func deleteByImage(db, table string, cols []string, row []any, pks []string) string {
	where, matched := imageWhere(cols, row, pks)
	if where == "" {
		return ""
	}
	if matched { // 主键定位，精确命中
		return fmt.Sprintf("DELETE FROM `%s`.`%s` WHERE %s;", db, table, where)
	}
	// 无主键只能全列匹配，加 LIMIT 1 避免连带删掉内容相同的其他行
	return fmt.Sprintf("DELETE FROM `%s`.`%s` WHERE %s LIMIT 1;", db, table, where)
}

// insertByImage 生成「撤销 DELETE」的语句：把行镜像插回去。
func insertByImage(db, table string, cols []string, row []any) string {
	if len(row) != len(cols) {
		return ""
	}
	names := make([]string, 0, len(cols))
	vals := make([]string, 0, len(cols))
	for i, c := range cols {
		names = append(names, "`"+c+"`")
		vals = append(vals, literal(row[i]))
	}
	return fmt.Sprintf("INSERT INTO `%s`.`%s` (%s) VALUES (%s);",
		db, table, strings.Join(names, ", "), strings.Join(vals, ", "))
}

// updateByImage 生成「撤销 UPDATE」的语句：把 before 与 after 有差异的列改回去。
func updateByImage(db, table string, cols []string, before, after []any, pks []string) string {
	if len(before) != len(cols) || len(after) != len(cols) {
		return ""
	}
	sets := make([]string, 0, len(cols))
	for i, c := range cols {
		if sameValue(before[i], after[i]) {
			continue
		}
		sets = append(sets, fmt.Sprintf("`%s` = %s", c, literal(before[i])))
	}
	if len(sets) == 0 {
		return ""
	}
	// 定位当前行用 after 镜像（主键被改过也成立）
	where, matched := imageWhere(cols, after, pks)
	if where == "" {
		return ""
	}
	q := fmt.Sprintf("UPDATE `%s`.`%s` SET %s WHERE %s", db, table, strings.Join(sets, ", "), where)
	if !matched {
		q += " LIMIT 1"
	}
	return q + ";"
}

// imageWhere 依据行镜像生成定位条件。
// 返回 matched=false 表示退化成全列匹配（无主键），调用方需加 LIMIT 1。
func imageWhere(cols []string, row []any, pks []string) (string, bool) {
	if len(row) != len(cols) {
		return "", false
	}
	if len(pks) > 0 {
		conds := make([]string, 0, len(pks))
		for _, pk := range pks {
			i := indexFold(cols, pk)
			if i < 0 {
				return "", false
			}
			conds = append(conds, fmt.Sprintf("`%s` = %s", pk, literal(row[i])))
		}
		return strings.Join(conds, " AND "), true
	}
	conds := make([]string, 0, len(cols))
	for i, c := range cols {
		// 用 NULL 安全等于：NULL 列也要能匹配上
		conds = append(conds, fmt.Sprintf("`%s` <=> %s", c, literal(row[i])))
	}
	return strings.Join(conds, " AND "), false
}

// sameValue 判断两列值是否相同（按渲染成 SQL 字面量的结果比较）。
func sameValue(a, b any) bool { return literal(a) == literal(b) }

func isWriteRows(t replication.EventType) bool {
	switch t {
	case replication.WRITE_ROWS_EVENTv0, replication.WRITE_ROWS_EVENTv1, replication.WRITE_ROWS_EVENTv2:
		return true
	}
	return false
}

func isDeleteRows(t replication.EventType) bool {
	switch t {
	case replication.DELETE_ROWS_EVENTv0, replication.DELETE_ROWS_EVENTv1, replication.DELETE_ROWS_EVENTv2:
		return true
	}
	return false
}

func isUpdateRows(t replication.EventType) bool {
	switch t {
	case replication.UPDATE_ROWS_EVENTv0, replication.UPDATE_ROWS_EVENTv1, replication.UPDATE_ROWS_EVENTv2:
		return true
	}
	return false
}

// targetTables 解析出工单涉及的表（"db.table"）：窗口内可能有其他会话的写入，按表过滤掉。
func targetTables(schema string, stmts []string) map[string]bool {
	out := map[string]bool{}
	collect := func(expr ast.TableExpr, schema string) {}
	collect = func(expr ast.TableExpr, schema string) {
		switch n := expr.(type) {
		case *ast.TableRef:
			db := n.Schema
			if db == "" {
				db = schema
			}
			if db != "" && n.Name != "" {
				out[db+"."+n.Name] = true
			}
		case *ast.JoinClause:
			collect(n.Left, schema)
			collect(n.Right, schema)
		}
	}
	for _, text := range stmts {
		parsed, err := base.ParseStatements(storepb.Engine_MYSQL, text)
		if err != nil || len(parsed) == 0 {
			continue
		}
		node, ok := mysqlparser.GetOmniNode(parsed[0].AST)
		if !ok {
			continue
		}
		switch n := node.(type) {
		case *ast.InsertStmt:
			if n.Table != nil {
				collect(n.Table, schema)
			}
		case *ast.UpdateStmt:
			for _, e := range n.Tables {
				collect(e, schema)
			}
		case *ast.DeleteStmt:
			for _, e := range n.Tables {
				collect(e, schema)
			}
			for _, e := range n.Using {
				collect(e, schema)
			}
		}
	}
	return out
}
