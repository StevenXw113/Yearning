package dorischeck

import (
	"regexp"
	"strconv"
	"strings"
)

// CreateTable 一条 Doris 建表语句的关键信息（文本级提取，不做完整语法树）。
type CreateTable struct {
	Raw       string // 原始语句
	TableName string

	// 分桶
	HasDistribution bool   // 是否有 DISTRIBUTED BY（Doris 允许不写，会默认随机分桶）
	Distribution    string // hash / random，未写为空
	Buckets         int    // BUCKETS 数量，未写为 0

	// 副本
	ReplicationNum int // PROPERTIES 里的 replication_num，未写为 0

	// 数据模型
	KeyModel string // duplicate / unique / aggregate，未写为空（Doris 默认 duplicate）
}

var (
	reCreateTable = regexp.MustCompile(`(?i)^\s*create\s+table\s+(?:if\s+not\s+exists\s+)?` + "`?" + `([\w.]+)`)
	reDistributed = regexp.MustCompile(`(?i)\bdistributed\s+by\s+(hash|random)\b`)
	reBuckets     = regexp.MustCompile(`(?i)\bbuckets\s+(\d+)\b`)
	reReplication = regexp.MustCompile(`(?i)"?replication_num"?\s*=\s*"?(\d+)"?`)
	reKeyModel    = regexp.MustCompile(`(?i)\b(duplicate|unique|aggregate)\s+key\b`)
)

// Parse 解析 Doris 建表语句；不是 CREATE TABLE 返回 nil。
//
// 只做文本级提取：Doris 建表语法用正则已经够定位到「有没有 DISTRIBUTED BY、
// BUCKETS 多少、副本数多少、什么 KEY 模型」这几个审核点，不必建完整语法树。
func Parse(sql string) *CreateTable {
	if !strings.Contains(strings.ToUpper(strings.TrimSpace(sql)), "CREATE TABLE") {
		return nil
	}
	m := reCreateTable.FindStringSubmatch(sql)
	if m == nil {
		return nil
	}
	stmt := &CreateTable{Raw: sql, TableName: m[1]}

	if dm := reDistributed.FindStringSubmatch(sql); dm != nil {
		stmt.HasDistribution = true
		stmt.Distribution = strings.ToLower(dm[1])
	}
	if bm := reBuckets.FindStringSubmatch(sql); bm != nil {
		if n, err := strconv.Atoi(bm[1]); err == nil {
			stmt.Buckets = n
		}
	}
	if rm := reReplication.FindStringSubmatch(sql); rm != nil {
		if n, err := strconv.Atoi(rm[1]); err == nil {
			stmt.ReplicationNum = n
		}
	}
	if km := reKeyModel.FindStringSubmatch(sql); km != nil {
		stmt.KeyModel = strings.ToLower(km[1])
	}
	return stmt
}
