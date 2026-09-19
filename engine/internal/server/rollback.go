package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bytebase/omni/mysql/ast"

	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/parser/base"
	mysqlparser "engine/internal/bytebase/plugin/parser/mysql"
)

// rollbackMaxRows 限制单条语句抓取的前镜像行数，避免大表全表扫描与结果集膨胀。
const rollbackMaxRows = 1000

// rowQueryer 抽象取数连接：事务模式下用 *sql.Tx，普通模式用 *sql.DB，
// 保证前镜像读到的是同一事务内的最新状态。
type rowQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// rollbackBuilder 在 DML 执行前抓取前镜像（pre-image），据此生成行级回滚语句。
//
// 不依赖 binlog 或备份表，只覆盖能安全推导的场景：
//   - 单表 UPDATE / DELETE：要求带 WHERE、表有主键、UPDATE 不改主键
//   - INSERT：要求显式给出全部主键列的字面量值
//
// 其余场景（多表、无 WHERE 的全表操作、无主键、INSERT...SELECT/SET、主键被改写）返回空串：
// 宁可不给回滚语句，也不给一条会写坏数据的反向语句。
type rollbackBuilder struct {
	*tableMeta
}

func newRollbackBuilder(q rowQueryer, schema string) *rollbackBuilder {
	return &rollbackBuilder{tableMeta: newTableMeta(q, schema)}
}

// build 返回单条语句的回滚 SQL；任何异常都返回空串，绝不影响执行本身。
func (b *rollbackBuilder) build(ctx context.Context, text string) string {
	parsed, err := base.ParseStatements(storepb.Engine_MYSQL, text)
	if err != nil || len(parsed) == 0 {
		return ""
	}
	node, ok := mysqlparser.GetOmniNode(parsed[0].AST)
	if !ok {
		return ""
	}
	switch n := node.(type) {
	case *ast.UpdateStmt:
		return b.update(ctx, text, n)
	case *ast.DeleteStmt:
		return b.delete(ctx, text, n)
	case *ast.InsertStmt:
		return b.insert(ctx, n)
	}
	return ""
}

// update 生成 UPDATE 的回滚：按前镜像逐行还原为改动前的值。
func (b *rollbackBuilder) update(ctx context.Context, text string, n *ast.UpdateStmt) string {
	tbl, ok := singleTable(n.Tables)
	if !ok || n.Where == nil {
		return ""
	}
	db, pks := b.primaryKeysOf(ctx, tbl)
	if db == "" || len(pks) == 0 {
		return ""
	}
	for _, a := range n.SetList { // 主键被改写后旧主键定位不到原行，放弃
		if a.Column != nil && containsFold(pks, a.Column.Column) {
			return ""
		}
	}
	rows, cols, ok := b.preImage(ctx, text, db, tbl)
	if !ok {
		return ""
	}
	var out []string
	for _, row := range rows {
		sets := make([]string, 0, len(cols))
		for i, c := range cols {
			if containsFold(pks, c) {
				continue
			}
			sets = append(sets, fmt.Sprintf("`%s` = %s", c, literal(row[i])))
		}
		if len(sets) == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("UPDATE `%s`.`%s` SET %s WHERE %s;",
			db, tbl.Name, strings.Join(sets, ", "), pkWhere(pks, cols, row)))
	}
	return strings.Join(out, "\n")
}

// delete 生成 DELETE 的回滚：把前镜像逐行重新 INSERT 回去。
func (b *rollbackBuilder) delete(ctx context.Context, text string, n *ast.DeleteStmt) string {
	tbl, ok := singleTable(n.Tables)
	if !ok || n.Where == nil || len(n.Using) > 0 {
		return ""
	}
	db, pks := b.primaryKeysOf(ctx, tbl)
	if db == "" || len(pks) == 0 {
		return ""
	}
	rows, cols, ok := b.preImage(ctx, text, db, tbl)
	if !ok {
		return ""
	}
	var out []string
	for _, row := range rows {
		names := make([]string, 0, len(cols))
		vals := make([]string, 0, len(cols))
		for i, c := range cols {
			names = append(names, "`"+c+"`")
			vals = append(vals, literal(row[i]))
		}
		out = append(out, fmt.Sprintf("INSERT INTO `%s`.`%s` (%s) VALUES (%s);",
			db, tbl.Name, strings.Join(names, ", "), strings.Join(vals, ", ")))
	}
	return strings.Join(out, "\n")
}

// insert 生成 INSERT 的回滚：按语句里显式给出的主键字面量删除这些新行。
func (b *rollbackBuilder) insert(ctx context.Context, n *ast.InsertStmt) string {
	if n.Select != nil || n.TableSource != nil || len(n.SetList) > 0 || len(n.Values) == 0 || len(n.Columns) == 0 {
		return ""
	}
	db, pks := b.primaryKeysOf(ctx, n.Table)
	if db == "" || len(pks) == 0 {
		return ""
	}
	idx := map[string]int{}
	for i, c := range n.Columns {
		if c != nil {
			idx[strings.ToLower(c.Column)] = i
		}
	}
	for _, pk := range pks {
		if _, ok := idx[strings.ToLower(pk)]; !ok { // 主键没显式写，取不到新行主键
			return ""
		}
	}
	var out []string
	for _, row := range n.Values {
		conds := make([]string, 0, len(pks))
		for _, pk := range pks {
			i := idx[strings.ToLower(pk)]
			if i >= len(row) {
				return ""
			}
			v, ok := literalFromExpr(row[i])
			if !ok { // 函数调用/子查询等拼不出安全的定位条件
				return ""
			}
			conds = append(conds, fmt.Sprintf("`%s` = %s", pk, v))
		}
		out = append(out, fmt.Sprintf("DELETE FROM `%s`.`%s` WHERE %s;", db, n.Table.Name, strings.Join(conds, " AND ")))
	}
	return strings.Join(out, "\n")
}

// preImage 用原语句 WHERE 起的原文作为条件，抓取将被改动的行。
// 条件原文直接复用（含 ORDER BY/LIMIT），保证抓到的正是这条语句将改动的行。
func (b *rollbackBuilder) preImage(ctx context.Context, text, db string, tbl *ast.TableRef) ([][]any, []string, bool) {
	cond, ok := topLevelWhere(text)
	if !ok {
		return nil, nil, false
	}
	if !hasLimit(cond) {
		cond += fmt.Sprintf(" LIMIT %d", rollbackMaxRows)
	}
	q := fmt.Sprintf("SELECT * FROM %s WHERE %s", tableRefSQL(db, tbl), cond)
	rows, err := b.q.QueryContext(ctx, q)
	if err != nil {
		return nil, nil, false
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, false
	}
	var data [][]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, false
		}
		data = append(data, vals)
	}
	if rows.Err() != nil {
		return nil, nil, false
	}
	return data, cols, true
}

// tableMeta 按需查询并缓存表结构信息（主键列、列顺序）。
// 列顺序很关键：binlog 行镜像按表定义顺序排列，需要按序号映射回列名。
type tableMeta struct {
	q        rowQueryer
	schema   string
	pkCache  map[string][]string
	colCache map[string][]string
}

func newTableMeta(q rowQueryer, schema string) *tableMeta {
	return &tableMeta{q: q, schema: schema, pkCache: map[string][]string{}, colCache: map[string][]string{}}
}

// resolve 补全库名：语句未写库名时用连接的默认库。
func (m *tableMeta) resolve(db, table string) (string, string, bool) {
	if db == "" {
		db = m.schema
	}
	if db == "" || table == "" {
		return "", "", false
	}
	return db, table, true
}

// primaryKeysOf 返回语句里表的库名与主键列；主键缺失表示该表没有主键。
func (m *tableMeta) primaryKeysOf(ctx context.Context, tbl *ast.TableRef) (string, []string) {
	if tbl == nil {
		return "", nil
	}
	db, table, ok := m.resolve(tbl.Schema, tbl.Name)
	if !ok {
		return "", nil
	}
	return db, m.primaryKeys(ctx, db, table)
}

// primaryKeys 查表主键列（带缓存）。
func (m *tableMeta) primaryKeys(ctx context.Context, db, table string) []string {
	key := db + "." + table
	if pks, ok := m.pkCache[key]; ok {
		return pks
	}
	pks := m.columnQuery(ctx,
		"SELECT COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND INDEX_NAME = 'PRIMARY' ORDER BY SEQ_IN_INDEX",
		db, table)
	m.pkCache[key] = pks
	return pks
}

// columns 按表定义顺序返回列名（带缓存），用于 binlog 行镜像的按序映射。
func (m *tableMeta) columns(ctx context.Context, db, table string) []string {
	key := db + "." + table
	if cols, ok := m.colCache[key]; ok {
		return cols
	}
	cols := m.columnQuery(ctx,
		"SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION",
		db, table)
	m.colCache[key] = cols
	return cols
}

func (m *tableMeta) columnQuery(ctx context.Context, query, db, table string) []string {
	var out []string
	rows, err := m.q.QueryContext(ctx, query, db, table)
	if err != nil {
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err == nil {
			out = append(out, c)
		}
	}
	return out
}

func singleTable(exprs []ast.TableExpr) (*ast.TableRef, bool) {
	if len(exprs) != 1 {
		return nil, false
	}
	tbl, ok := exprs[0].(*ast.TableRef)
	return tbl, ok
}

func tableRefSQL(db string, tbl *ast.TableRef) string {
	q := fmt.Sprintf("`%s`.`%s`", db, tbl.Name)
	if tbl.Alias != "" {
		q += fmt.Sprintf(" AS `%s`", tbl.Alias)
	}
	return q
}

func pkWhere(pks, cols []string, row []any) string {
	conds := make([]string, 0, len(pks))
	for _, pk := range pks {
		i := indexFold(cols, pk)
		if i < 0 {
			continue
		}
		conds = append(conds, fmt.Sprintf("`%s` = %s", pk, literal(row[i])))
	}
	return strings.Join(conds, " AND ")
}

// topLevelWhere 返回语句顶层 WHERE 之后的原文（去掉结尾分号）。
// 引号内文本与括号内的子查询会被跳过，避免把子查询的 WHERE 当成本条语句的条件。
func topLevelWhere(text string) (string, bool) {
	depth, pos := 0, -1
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		if quote != 0 {
			switch {
			case c == '\\':
				i++
			case c == quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			quote = c
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && keywordAt(text, i, "WHERE") {
				pos = i
			}
		}
	}
	if pos < 0 {
		return "", false
	}
	cond := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text[pos+len("WHERE"):]), ";"))
	if cond == "" {
		return "", false
	}
	return cond, true
}

// keywordAt 判断 text[i:] 是否以 keyword 开头且两侧是词边界。
func keywordAt(text string, i int, keyword string) bool {
	if i+len(keyword) > len(text) || !strings.EqualFold(text[i:i+len(keyword)], keyword) {
		return false
	}
	if i > 0 && isWordByte(text[i-1]) {
		return false
	}
	if end := i + len(keyword); end < len(text) && isWordByte(text[end]) {
		return false
	}
	return true
}

func isWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// literal 把查询结果值渲染成 SQL 字面量。
func literal(v any) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		if utf8.Valid(x) && !bytes.ContainsRune(x, 0) {
			return quote(string(x))
		}
		return "0x" + hex.EncodeToString(x)
	case string:
		return quote(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case int8:
		return strconv.FormatInt(int64(x), 10)
	case int16:
		return strconv.FormatInt(int64(x), 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint:
		return strconv.FormatUint(uint64(x), 10)
	case uint8:
		return strconv.FormatUint(uint64(x), 10)
	case uint16:
		return strconv.FormatUint(uint64(x), 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		if x {
			return "1"
		}
		return "0"
	case time.Time:
		return quote(x.Format("2006-01-02 15:04:05"))
	default:
		return quote(fmt.Sprint(x))
	}
}

// literalFromExpr 把 INSERT 语句里的字面量渲染成 SQL 字面量。
func literalFromExpr(n ast.ExprNode) (string, bool) {
	switch x := n.(type) {
	case *ast.IntLit:
		return strconv.FormatInt(x.Value, 10), true
	case *ast.FloatLit:
		return x.Value, true
	case *ast.StringLit:
		return quote(x.Value), true
	case *ast.BoolLit:
		if x.Value {
			return "1", true
		}
		return "0", true
	case *ast.NullLit:
		return "NULL", true
	}
	return "", false
}

func quote(s string) string {
	return "'" + strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(s) + "'"
}

func containsFold(list []string, s string) bool { return indexFold(list, s) >= 0 }

func indexFold(list []string, s string) int {
	for i, v := range list {
		if strings.EqualFold(v, s) {
			return i
		}
	}
	return -1
}

func hasLimit(s string) bool {
	for _, f := range strings.Fields(s) {
		if strings.EqualFold(f, "limit") {
			return true
		}
	}
	return false
}
