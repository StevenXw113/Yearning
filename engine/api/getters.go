// Package api 的取值方法:与旧 gRPC 生成代码同形态的 nil 安全 getter。
// 引擎实现层(rules.go/check.go 等)大量使用 GetX 风格,且 proto getter 对 nil
// 接收者返回零值——本文件保留该语义,调用面在协议切换中零改动。
// 由 api.go 字段机械生成;新增契约字段时同步补一行即可。
package api

func (r *DataSource) GetKind() string {
	if r == nil {
		return ""
	}
	return r.Kind
}
func (r *AuditRole) GetDmlAllowLimitStmt() bool {
	if r == nil {
		return false
	}
	return r.DmlAllowLimitStmt
}
func (r *AuditRole) GetDmlInsertColumns() bool {
	if r == nil {
		return false
	}
	return r.DmlInsertColumns
}
func (r *AuditRole) GetDmlMaxInsertRows() int32 {
	if r == nil {
		return 0
	}
	return r.DmlMaxInsertRows
}
func (r *AuditRole) GetDmlWhere() bool {
	if r == nil {
		return false
	}
	return r.DmlWhere
}
func (r *AuditRole) GetDmlWhereExprValueIsNull() bool {
	if r == nil {
		return false
	}
	return r.DmlWhereExprValueIsNull
}
func (r *AuditRole) GetDmlOrder() bool {
	if r == nil {
		return false
	}
	return r.DmlOrder
}
func (r *AuditRole) GetDmlInsertMustExplicitly() bool {
	if r == nil {
		return false
	}
	return r.DmlInsertMustExplicitly
}
func (r *AuditRole) GetDdlEnablePrimaryKey() bool {
	if r == nil {
		return false
	}
	return r.DdlEnablePrimaryKey
}
func (r *AuditRole) GetDdlCheckTableComment() bool {
	if r == nil {
		return false
	}
	return r.DdlCheckTableComment
}
func (r *AuditRole) GetDdlCheckColumnComment() bool {
	if r == nil {
		return false
	}
	return r.DdlCheckColumnComment
}
func (r *AuditRole) GetDdlCheckColumnDefault() bool {
	if r == nil {
		return false
	}
	return r.DdlCheckColumnDefault
}
func (r *AuditRole) GetDdlEnableAutoincrementInit() bool {
	if r == nil {
		return false
	}
	return r.DdlEnableAutoincrementInit
}
func (r *AuditRole) GetDdlEnableAutoincrementUnsigned() bool {
	if r == nil {
		return false
	}
	return r.DdlEnableAutoincrementUnsigned
}
func (r *AuditRole) GetDdlEnableDropTable() bool {
	if r == nil {
		return false
	}
	return r.DdlEnableDropTable
}
func (r *AuditRole) GetDdlEnableDropDatabase() bool {
	if r == nil {
		return false
	}
	return r.DdlEnableDropDatabase
}
func (r *AuditRole) GetDdlIndexNameSpec() bool {
	if r == nil {
		return false
	}
	return r.DdlIndexNameSpec
}
func (r *AuditRole) GetDdlMaxKeyParts() uint32 {
	if r == nil {
		return 0
	}
	return r.DdlMaxKeyParts
}
func (r *AuditRole) GetDdlMaxCharLength() uint32 {
	if r == nil {
		return 0
	}
	return r.DdlMaxCharLength
}
func (r *AuditRole) GetMaxTableNameLen() int32 {
	if r == nil {
		return 0
	}
	return r.MaxTableNameLen
}
func (r *AuditRole) GetMaxAffectRows() uint32 {
	if r == nil {
		return 0
	}
	return r.MaxAffectRows
}
func (r *AuditRole) GetMaxDdlAffectRows() uint32 {
	if r == nil {
		return 0
	}
	return r.MaxDdlAffectRows
}
func (r *AuditRole) GetSupportCharset() string {
	if r == nil {
		return ""
	}
	return r.SupportCharset
}
func (r *AuditRole) GetSupportCollation() string {
	if r == nil {
		return ""
	}
	return r.SupportCollation
}
func (r *AuditRole) GetCheckIdentifier() bool {
	if r == nil {
		return false
	}
	return r.CheckIdentifier
}
func (r *AuditRole) GetMustHaveColumns() string {
	if r == nil {
		return ""
	}
	return r.MustHaveColumns
}
func (r *AuditRole) GetDdlMultiToCommit() bool {
	if r == nil {
		return false
	}
	return r.DdlMultiToCommit
}
func (r *AuditRole) GetDdlAllowColumnType() bool {
	if r == nil {
		return false
	}
	return r.DdlAllowColumnType
}
func (r *AuditRole) GetDdlAllowPriNotInt() bool {
	if r == nil {
		return false
	}
	return r.DdlAllowPriNotInt
}
func (r *AuditRole) GetDdlAllowMultiAlter() bool {
	if r == nil {
		return false
	}
	return r.DdlAllowMultiAlter
}
func (r *AuditRole) GetDdlEnableForeignKey() bool {
	if r == nil {
		return false
	}
	return r.DdlEnableForeignKey
}
func (r *AuditRole) GetDdlTablePrefix() string {
	if r == nil {
		return ""
	}
	return r.DdlTablePrefix
}
func (r *AuditRole) GetDdlAllowChangeColumnPosition() bool {
	if r == nil {
		return false
	}
	return r.DdlAllowChangeColumnPosition
}
func (r *AuditRole) GetDdlCheckFloatDouble() bool {
	if r == nil {
		return false
	}
	return r.DdlCheckFloatDouble
}
func (r *AuditRole) GetAllowCreateView() bool {
	if r == nil {
		return false
	}
	return r.AllowCreateView
}
func (r *AuditRole) GetAllowCreatePartition() bool {
	if r == nil {
		return false
	}
	return r.AllowCreatePartition
}
func (r *AuditRole) GetAllowSpecialType() bool {
	if r == nil {
		return false
	}
	return r.AllowSpecialType
}
func (r *AuditRole) GetRuleLevels() map[string]string {
	if r == nil {
		return nil
	}
	return r.RuleLevels
}
func (r *AuditRole) GetDdlForbidTruncate() bool {
	if r == nil {
		return false
	}
	return r.DdlForbidTruncate
}
func (r *Order) GetStatus() uint32 {
	if r == nil {
		return 0
	}
	return r.Status
}
func (r *Order) GetType() int32 {
	if r == nil {
		return 0
	}
	return r.Type
}
func (r *Order) GetSource() string {
	if r == nil {
		return ""
	}
	return r.Source
}
func (r *Order) GetTable() string {
	if r == nil {
		return ""
	}
	return r.Table
}
func (r *Record) GetStatus() string {
	if r == nil {
		return ""
	}
	return r.Status
}
func (r *Record) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}
func (r *Record) GetLevel() uint32 {
	if r == nil {
		return 0
	}
	return r.Level
}
func (r *Record) GetTable() string {
	if r == nil {
		return ""
	}
	return r.Table
}
func (r *Record) GetSchema() string {
	if r == nil {
		return ""
	}
	return r.Schema
}
func (r *CheckRequest) GetSchema() string {
	if r == nil {
		return ""
	}
	return r.Schema
}
func (r *CheckRequest) GetMode() string {
	if r == nil {
		return ""
	}
	return r.Mode
}
func (r *CheckReply) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}
func (r *MergeAlterTablesReply) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}
func (r *ExecRequest) GetMaxAffectRows() uint32 {
	if r == nil {
		return 0
	}
	return r.MaxAffectRows
}
func (r *ExecReply) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}
func (r *QueryRequest) GetLimit() uint64 {
	if r == nil {
		return 0
	}
	return r.Limit
}
func (r *QueryRequest) GetKind() string {
	if r == nil {
		return ""
	}
	return r.Kind
}
func (r *QueryReply) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}
