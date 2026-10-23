package dorischeck

// Check 审核一条 Doris 建表语句，返回命中的规则。
// sql 不是 CREATE TABLE 时返回 nil（调用方应走 MySQL 解析器审核 DML/查询）。
func Check(sql string, cfg Config) []Finding {
	stmt := Parse(sql)
	if stmt == nil {
		return nil
	}
	var out []Finding
	for _, r := range All() {
		out = append(out, r.Check(cfg, stmt)...)
	}
	return out
}
