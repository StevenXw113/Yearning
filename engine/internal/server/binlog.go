package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bytebase/omni/mysql/ast"
	"github.com/go-mysql-org/go-mysql/mysql"
	"github.com/go-mysql-org/go-mysql/replication"

	"engine/api"
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/parser/base"
	mysqlparser "engine/internal/bytebase/plugin/parser/mysql"
)

// binlogCaptureTimeout 是等待 binlog 事件的上限：超过就用手头已有的事件生成回滚，不阻塞执行。
const binlogCaptureTimeout = 10 * time.Second

// binlogServerBase / binlogServerSeq 进程内唯一的复制 server_id 分配：
// 基数进程启动时随机取（避开源库自身与常见从库段），叠加自增序号。
// 此前用「纳秒时间戳取模」，同源库并发工单可能撞号：MySQL 会踢掉同 server_id
// 的已有复制连接，先建立的抓取静默断流、该工单回滚丢失。
var (
	binlogServerBase = uint32(1000000 + rand.Int63n(900000000))
	binlogServerSeq  atomic.Uint64
)

func nextBinlogServerID() uint32 {
	return binlogServerBase + uint32(binlogServerSeq.Add(1)%900000000)
}

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
func newBinlogCapture(ctx context.Context, src *api.DataSource, db *sql.DB, meta *tableMeta, schema string, stmts []string) (*binlogCapture, bool) {
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
	cfg := replication.BinlogSyncerConfig{
		// server_id 需与源库及其他从库不同，进程内唯一分配（见 nextBinlogServerID）
		ServerID: nextBinlogServerID(),
		Flavor:   "mysql",
		Host:     src.Ip,
		Port:     uint16(src.Port),
		User:     src.Username,
		Password: src.Password,
		Charset:  "utf8mb4",
	}
	// 复制流与 SQL 连接同源：数据源配了 TLS 时复制流也走 TLS，
	// 否则口令与整段行数据在复制连接上明文传输
	if tc := binlogTLSConfig(src); tc != nil {
		cfg.TLSConfig = tc
	}
	return &binlogCapture{
		cfg:    cfg,
		meta:   meta,
		tables: tables,
		start:  start,
	}, true
}

// binlogTLSConfig 按 DataSource 的 CA / 客户端证书构造复制流 TLS 配置。
// CA 或证书对非法（解析失败）时返回 nil：退回明文与旧行为一致，由抓取失败路径兜底。
func binlogTLSConfig(src *api.DataSource) *tls.Config {
	if src.Ca == "" && src.Cert == "" && src.Key == "" {
		return nil
	}
	cfg := &tls.Config{ServerName: src.Ip}
	if src.Ca != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(src.Ca)) {
			return nil
		}
		cfg.RootCAs = pool
	}
	if src.Cert != "" && src.Key != "" {
		cert, err := tls.X509KeyPair([]byte(src.Cert), []byte(src.Key))
		if err != nil {
			return nil
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg
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
// 返回的 warning 非空表示回滚语句缺失或不完整（连接失败 / 超时 / 行镜像与
// 表结构对不上），调用方必须把它呈现给审批人，不能静默吞掉。
func (c *binlogCapture) rollback(ctx context.Context, end mysql.Position) ([]string, string) {
	if c.start.Name == "" || end.Name == "" {
		return nil, ""
	}
	syncer := replication.NewBinlogSyncer(c.cfg)
	defer syncer.Close()
	streamer, err := syncer.StartSync(c.start)
	if err != nil {
		return nil, "回滚抓取失败：无法建立复制连接（" + err.Error() + "），本工单没有自动回滚语句"
	}
	ctx, cancel := context.WithTimeout(ctx, binlogCaptureTimeout)
	defer cancel()

	cur := c.start.Name
	var stmts []string
	skipped := 0
	for {
		ev, err := streamer.GetEvent(ctx)
		if err != nil {
			// 超时或连接中断：用手头已有的事件，但必须告知回滚可能不完整
			w := "回滚抓取中断（" + err.Error() + "），回滚语句可能不完整，请人工确认"
			if ctx.Err() != nil {
				w = "回滚抓取超时（" + binlogCaptureTimeout.String() + "），回滚语句可能不完整，请人工确认"
			}
			stmts = reverseStmts(stmts)
			return stmts, joinSkipWarning(w, skipped)
		}
		switch e := ev.Event.(type) {
		case *replication.RotateEvent:
			cur = string(e.NextLogName)
		case *replication.RowsEvent:
			s, sk := c.rowsRollback(ctx, ev.Header.EventType, e)
			stmts = append(stmts, s...)
			skipped += sk
		}
		if cur > end.Name || (cur == end.Name && ev.Header.LogPos >= end.Pos) {
			break
		}
	}
	stmts = reverseStmts(stmts) // 逆序执行才能正确撤销：最后一条改动最先还原
	return stmts, joinSkipWarning("", skipped)
}

// joinSkipWarning 合并「抓取异常」与「行级跳过」两类提示。
func joinSkipWarning(w string, skipped int) string {
	if skipped <= 0 {
		return w
	}
	sk := fmt.Sprintf("有 %d 行事件因与表结构不匹配未生成回滚语句，请人工确认", skipped)
	if w == "" {
		return sk
	}
	return w + "；" + sk
}

func reverseStmts(stmts []string) []string {
	for i, j := 0, len(stmts)-1; i < j; i, j = i+1, j-1 {
		stmts[i], stmts[j] = stmts[j], stmts[i]
	}
	return stmts
}

// rowsRollback 把一条行事件转成回滚语句；第二个返回值为因结构不匹配被跳过的行数。
func (c *binlogCapture) rowsRollback(ctx context.Context, t replication.EventType, e *replication.RowsEvent) ([]string, int) {
	if e.Table == nil {
		return nil, 0
	}
	db, table := string(e.Table.Schema), string(e.Table.Table)
	if !c.tables[db+"."+table] {
		return nil, 0 // 窗口内其他会话/其他表的写入不属于本工单
	}
	cols := c.meta.columns(ctx, db, table)
	if len(cols) == 0 {
		return nil, 0
	}
	pks := c.meta.primaryKeys(ctx, db, table)

	var out []string
	skipped := 0
	// 生成失败的行（列数与表结构不匹配等）必须计数上报，不能静默丢行
	emit := func(sql string) {
		if sql == "" {
			skipped++
			return
		}
		out = append(out, sql)
	}
	switch {
	case isWriteRows(t): // 插入的行：镜像即插入后的样子
		for _, row := range e.Rows {
			emit(deleteByImage(db, table, cols, row, pks))
		}
	case isDeleteRows(t): // 删除的行：镜像即删除前的内容，塞回去即可
		for _, row := range e.Rows {
			emit(insertByImage(db, table, cols, row))
		}
	case isUpdateRows(t): // 镜像成对出现：before, after, before, after...
		for i := 0; i+1 < len(e.Rows); i += 2 {
			emit(updateByImage(db, table, cols, e.Rows[i], e.Rows[i+1], pks))
		}
	}
	return out, skipped
}

// deleteByImage 生成「撤销 INSERT」的语句：按行镜像删掉这一行。
func deleteByImage(db, table string, cols []string, row []any, pks []string) string {
	where, matched := imageWhere(cols, row, pks)
	if where == "" {
		return ""
	}
	target := quoteIdent(db) + "." + quoteIdent(table)
	if matched { // 主键定位，精确命中
		return fmt.Sprintf("DELETE FROM %s WHERE %s;", target, where)
	}
	// 无主键只能全列匹配，加 LIMIT 1 避免连带删掉内容相同的其他行
	return fmt.Sprintf("DELETE FROM %s WHERE %s LIMIT 1;", target, where)
}

// insertByImage 生成「撤销 DELETE」的语句：把行镜像插回去。
func insertByImage(db, table string, cols []string, row []any) string {
	if len(row) != len(cols) {
		return ""
	}
	names := make([]string, 0, len(cols))
	vals := make([]string, 0, len(cols))
	for i, c := range cols {
		names = append(names, quoteIdent(c))
		vals = append(vals, literal(row[i]))
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		quoteIdent(db)+"."+quoteIdent(table), strings.Join(names, ", "), strings.Join(vals, ", "))
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
		sets = append(sets, fmt.Sprintf("%s = %s", quoteIdent(c), literal(before[i])))
	}
	if len(sets) == 0 {
		return ""
	}
	// 定位当前行用 after 镜像（主键被改过也成立）
	where, matched := imageWhere(cols, after, pks)
	if where == "" {
		return ""
	}
	q := fmt.Sprintf("UPDATE %s SET %s WHERE %s", quoteIdent(db)+"."+quoteIdent(table), strings.Join(sets, ", "), where)
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
			conds = append(conds, fmt.Sprintf("%s = %s", quoteIdent(pk), literal(row[i])))
		}
		return strings.Join(conds, " AND "), true
	}
	conds := make([]string, 0, len(cols))
	for i, c := range cols {
		// 用 NULL 安全等于：NULL 列也要能匹配上
		conds = append(conds, fmt.Sprintf("%s <=> %s", quoteIdent(c), literal(row[i])))
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
