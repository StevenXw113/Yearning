// Package api 的取值方法:与旧 gRPC 生成代码同形态的 nil 安全 getter。
// 引擎实现层(rules.go/check.go 等)大量使用 GetX 风格,且 proto getter 对 nil
// 接收者返回零值——本文件保留该语义,调用面在协议切换中零改动。
// 由 api.go 字段机械生成;新增契约字段时同步补一行即可。
package api

func (r *DataSource) GetIp() string {
	if r == nil {
		return ""
	}
	return r.Ip
}

func (r *DataSource) GetPort() int32 {
	if r == nil {
		return 0
	}
	return r.Port
}

func (r *DataSource) GetUsername() string {
	if r == nil {
		return ""
	}
	return r.Username
}

func (r *DataSource) GetPassword() string {
	if r == nil {
		return ""
	}
	return r.Password
}

func (r *DataSource) GetCa() string {
	if r == nil {
		return ""
	}
	return r.Ca
}

func (r *DataSource) GetCert() string {
	if r == nil {
		return ""
	}
	return r.Cert
}

func (r *DataSource) GetKey() string {
	if r == nil {
		return ""
	}
	return r.Key
}

func (r *DataSource) GetKind() string {
	if r == nil {
		return ""
	}
	return r.Kind
}

func (r *DataSource) GetAuthDb() string {
	if r == nil {
		return ""
	}
	return r.AuthDb
}

func (r *AuditRole) GetDmlTransaction() bool {
	if r == nil {
		return false
	}
	return r.DmlTransaction
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

func (r *AuditRole) GetDmlSelect() bool {
	if r == nil {
		return false
	}
	return r.DmlSelect
}

func (r *AuditRole) GetDmlAllowInsertNull() bool {
	if r == nil {
		return false
	}
	return r.DmlAllowInsertNull
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

func (r *AuditRole) GetDdlCheckColumnNullable() bool {
	if r == nil {
		return false
	}
	return r.DdlCheckColumnNullable
}

func (r *AuditRole) GetDdlCheckColumnDefault() bool {
	if r == nil {
		return false
	}
	return r.DdlCheckColumnDefault
}

func (r *AuditRole) GetDdlEnableAcrossDbRename() bool {
	if r == nil {
		return false
	}
	return r.DdlEnableAcrossDbRename
}

func (r *AuditRole) GetDdlEnableAutoincrementInit() bool {
	if r == nil {
		return false
	}
	return r.DdlEnableAutoincrementInit
}

func (r *AuditRole) GetDdlEnableAutoIncrement() bool {
	if r == nil {
		return false
	}
	return r.DdlEnableAutoIncrement
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

func (r *AuditRole) GetDdlEnableNullIndexName() bool {
	if r == nil {
		return false
	}
	return r.DdlEnableNullIndexName
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

func (r *AuditRole) GetDdlMaxKey() uint32 {
	if r == nil {
		return 0
	}
	return r.DdlMaxKey
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

func (r *AuditRole) GetDdlPrimaryKeyMust() bool {
	if r == nil {
		return false
	}
	return r.DdlPrimaryKeyMust
}

func (r *AuditRole) GetDdlAllowColumnType() bool {
	if r == nil {
		return false
	}
	return r.DdlAllowColumnType
}

func (r *AuditRole) GetDdlImplicitTypeConversion() bool {
	if r == nil {
		return false
	}
	return r.DdlImplicitTypeConversion
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

func (r *AuditRole) GetDdlColumnsMustHaveIndex() string {
	if r == nil {
		return ""
	}
	return r.DdlColumnsMustHaveIndex
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

func (r *AuditRole) GetIsOsc() bool {
	if r == nil {
		return false
	}
	return r.IsOsc
}

func (r *AuditRole) GetOscExpr() string {
	if r == nil {
		return ""
	}
	return r.OscExpr
}

func (r *AuditRole) GetOscSize() uint32 {
	if r == nil {
		return 0
	}
	return r.OscSize
}

func (r *AuditRole) GetAllowCreateView() bool {
	if r == nil {
		return false
	}
	return r.AllowCreateView
}

func (r *AuditRole) GetAllowCreateViewWithSelectStar() bool {
	if r == nil {
		return false
	}
	return r.AllowCreateViewWithSelectStar
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

func (r *AuditRole) GetPriRollBack() bool {
	if r == nil {
		return false
	}
	return r.PriRollBack
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

func (r *AuditRole) GetMongoForbidEmptyFilter() bool {
	if r == nil {
		return false
	}
	return r.MongoForbidEmptyFilter
}

func (r *AuditRole) GetMongoForbidDangerous() bool {
	if r == nil {
		return false
	}
	return r.MongoForbidDangerous
}

func (r *AuditRole) GetMongoForbidWhere() bool {
	if r == nil {
		return false
	}
	return r.MongoForbidWhere
}

func (r *AuditRole) GetMongoForbidDropCollection() bool {
	if r == nil {
		return false
	}
	return r.MongoForbidDropCollection
}

func (r *AuditRole) GetMongoMaxAffectRows() int32 {
	if r == nil {
		return 0
	}
	return r.MongoMaxAffectRows
}

func (r *AuditRole) GetMongoForbidSystemCollection() bool {
	if r == nil {
		return false
	}
	return r.MongoForbidSystemCollection
}

func (r *AuditRole) GetMongoForbidAdminCommand() bool {
	if r == nil {
		return false
	}
	return r.MongoForbidAdminCommand
}

func (r *AuditRole) GetMongoForbidImmutableId() bool {
	if r == nil {
		return false
	}
	return r.MongoForbidImmutableId
}

func (r *AuditRole) GetMongoForbidCappedConvert() bool {
	if r == nil {
		return false
	}
	return r.MongoForbidCappedConvert
}

func (r *AuditRole) GetMongoForbidCollMod() bool {
	if r == nil {
		return false
	}
	return r.MongoForbidCollMod
}

func (r *AuditRole) GetMongoIndexKeyLimit() int32 {
	if r == nil {
		return 0
	}
	return r.MongoIndexKeyLimit
}

func (r *AuditRole) GetMongoIndexNameSpec() string {
	if r == nil {
		return ""
	}
	return r.MongoIndexNameSpec
}

func (r *AuditRole) GetMongoCollectionPrefix() string {
	if r == nil {
		return ""
	}
	return r.MongoCollectionPrefix
}

func (r *AuditRole) GetMongoMaxCollectionNameLen() int32 {
	if r == nil {
		return 0
	}
	return r.MongoMaxCollectionNameLen
}

func (r *AuditRole) GetMongoRegexUnanchored() bool {
	if r == nil {
		return false
	}
	return r.MongoRegexUnanchored
}

func (r *AuditRole) GetMongoNegationOperator() bool {
	if r == nil {
		return false
	}
	return r.MongoNegationOperator
}

func (r *AuditRole) GetMongoOrClause() bool {
	if r == nil {
		return false
	}
	return r.MongoOrClause
}

func (r *AuditRole) GetMongoLargeInList() int32 {
	if r == nil {
		return 0
	}
	return r.MongoLargeInList
}

func (r *AuditRole) GetMongoQueryForbidNoFilter() bool {
	if r == nil {
		return false
	}
	return r.MongoQueryForbidNoFilter
}

func (r *AuditRole) GetMongoQueryForbidLookup() bool {
	if r == nil {
		return false
	}
	return r.MongoQueryForbidLookup
}

func (r *AuditRole) GetMongoQueryForbidNoLimit() bool {
	if r == nil {
		return false
	}
	return r.MongoQueryForbidNoLimit
}

func (r *AuditRole) GetRedisForbidDangerous() bool {
	if r == nil {
		return false
	}
	return r.RedisForbidDangerous
}

func (r *AuditRole) GetRedisForbidAdmin() bool {
	if r == nil {
		return false
	}
	return r.RedisForbidAdmin
}

func (r *AuditRole) GetRedisForbidScript() bool {
	if r == nil {
		return false
	}
	return r.RedisForbidScript
}

func (r *AuditRole) GetRedisForbidReplica() bool {
	if r == nil {
		return false
	}
	return r.RedisForbidReplica
}

func (r *AuditRole) GetRedisForbidKeysCommand() bool {
	if r == nil {
		return false
	}
	return r.RedisForbidKeysCommand
}

func (r *AuditRole) GetRedisForbidRename() bool {
	if r == nil {
		return false
	}
	return r.RedisForbidRename
}

func (r *AuditRole) GetRedisKeyPrefix() string {
	if r == nil {
		return ""
	}
	return r.RedisKeyPrefix
}

func (r *AuditRole) GetRedisMaxKeyLen() int32 {
	if r == nil {
		return 0
	}
	return r.RedisMaxKeyLen
}

func (r *AuditRole) GetRedisForbidMove() bool {
	if r == nil {
		return false
	}
	return r.RedisForbidMove
}

func (r *AuditRole) GetRedisMaxAffectKeys() int32 {
	if r == nil {
		return 0
	}
	return r.RedisMaxAffectKeys
}

func (r *AuditRole) GetRedisMaxValueLen() int32 {
	if r == nil {
		return 0
	}
	return r.RedisMaxValueLen
}

func (r *AuditRole) GetRedisRequireTtl() bool {
	if r == nil {
		return false
	}
	return r.RedisRequireTtl
}

func (r *AuditRole) GetRedisForbidPersist() bool {
	if r == nil {
		return false
	}
	return r.RedisForbidPersist
}

func (r *AuditRole) GetRedisScanRequireMatch() bool {
	if r == nil {
		return false
	}
	return r.RedisScanRequireMatch
}

func (r *AuditRole) GetRedisForbidBlocking() bool {
	if r == nil {
		return false
	}
	return r.RedisForbidBlocking
}

func (r *AuditRole) GetDorisRequireDistribution() bool {
	if r == nil {
		return false
	}
	return r.DorisRequireDistribution
}

func (r *AuditRole) GetDorisBucketLimit() int32 {
	if r == nil {
		return 0
	}
	return r.DorisBucketLimit
}

func (r *AuditRole) GetDorisReplicationNum() int32 {
	if r == nil {
		return 0
	}
	return r.DorisReplicationNum
}

func (r *AuditRole) GetDorisKeyModel() bool {
	if r == nil {
		return false
	}
	return r.DorisKeyModel
}

func (r *Order) GetWorkId() string {
	if r == nil {
		return ""
	}
	return r.WorkId
}

func (r *Order) GetUsername() string {
	if r == nil {
		return ""
	}
	return r.Username
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

func (r *Order) GetBackup() uint32 {
	if r == nil {
		return 0
	}
	return r.Backup
}

func (r *Order) GetIdc() string {
	if r == nil {
		return ""
	}
	return r.Idc
}

func (r *Order) GetSource() string {
	if r == nil {
		return ""
	}
	return r.Source
}

func (r *Order) GetSourceId() string {
	if r == nil {
		return ""
	}
	return r.SourceId
}

func (r *Order) GetDataBase() string {
	if r == nil {
		return ""
	}
	return r.DataBase
}

func (r *Order) GetTable() string {
	if r == nil {
		return ""
	}
	return r.Table
}

func (r *Order) GetDate() string {
	if r == nil {
		return ""
	}
	return r.Date
}

func (r *Order) GetSql() string {
	if r == nil {
		return ""
	}
	return r.Sql
}

func (r *Order) GetText() string {
	if r == nil {
		return ""
	}
	return r.Text
}

func (r *Order) GetAssigned() string {
	if r == nil {
		return ""
	}
	return r.Assigned
}

func (r *Order) GetDelay() string {
	if r == nil {
		return ""
	}
	return r.Delay
}

func (r *Order) GetRealName() string {
	if r == nil {
		return ""
	}
	return r.RealName
}

func (r *Order) GetExecuteTime() string {
	if r == nil {
		return ""
	}
	return r.ExecuteTime
}

func (r *Order) GetCurrentStep() int32 {
	if r == nil {
		return 0
	}
	return r.CurrentStep
}

func (r *Order) GetOscInfo() string {
	if r == nil {
		return ""
	}
	return r.OscInfo
}

func (r *Order) GetFile() string {
	if r == nil {
		return ""
	}
	return r.File
}

func (r *Record) GetSql() string {
	if r == nil {
		return ""
	}
	return r.Sql
}

func (r *Record) GetAffectRows() uint32 {
	if r == nil {
		return 0
	}
	return r.AffectRows
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

func (r *Record) GetExecTime() string {
	if r == nil {
		return ""
	}
	return r.ExecTime
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

func (r *Record) GetIsOsc() bool {
	if r == nil {
		return false
	}
	return r.IsOsc
}

func (r *Record) GetRollback() string {
	if r == nil {
		return ""
	}
	return r.Rollback
}

func (r *CheckRequest) GetSql() string {
	if r == nil {
		return ""
	}
	return r.Sql
}

func (r *CheckRequest) GetSchema() string {
	if r == nil {
		return ""
	}
	return r.Schema
}

func (r *CheckRequest) GetLang() string {
	if r == nil {
		return ""
	}
	return r.Lang
}

func (r *CheckRequest) GetMode() string {
	if r == nil {
		return ""
	}
	return r.Mode
}

func (r *CheckReply) GetOk() bool {
	if r == nil {
		return false
	}
	return r.Ok
}

func (r *CheckReply) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}

func (r *MergeAlterTablesRequest) GetSqls() string {
	if r == nil {
		return ""
	}
	return r.Sqls
}

func (r *MergeAlterTablesReply) GetOk() bool {
	if r == nil {
		return false
	}
	return r.Ok
}

func (r *MergeAlterTablesReply) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}

func (r *MergeAlterTablesReply) GetSql() string {
	if r == nil {
		return ""
	}
	return r.Sql
}

func (r *ExecRequest) GetMaxAffectRows() uint32 {
	if r == nil {
		return 0
	}
	return r.MaxAffectRows
}

func (r *ExecReply) GetOk() bool {
	if r == nil {
		return false
	}
	return r.Ok
}

func (r *ExecReply) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}

func (r *QueryRequest) GetSql() string {
	if r == nil {
		return ""
	}
	return r.Sql
}

func (r *QueryRequest) GetLimit() uint64 {
	if r == nil {
		return 0
	}
	return r.Limit
}

func (r *QueryRequest) GetInsulateWordList() string {
	if r == nil {
		return ""
	}
	return r.InsulateWordList
}

func (r *QueryRequest) GetKind() string {
	if r == nil {
		return ""
	}
	return r.Kind
}

func (r *QueryReply) GetOk() bool {
	if r == nil {
		return false
	}
	return r.Ok
}

func (r *QueryReply) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}

func (r *Confirm) GetWorkId() string {
	if r == nil {
		return ""
	}
	return r.WorkId
}

func (r *Confirm) GetPage() int32 {
	if r == nil {
		return 0
	}
	return r.Page
}

func (r *Confirm) GetFlag() int32 {
	if r == nil {
		return 0
	}
	return r.Flag
}

func (r *Confirm) GetText() string {
	if r == nil {
		return ""
	}
	return r.Text
}

func (r *Confirm) GetTp() string {
	if r == nil {
		return ""
	}
	return r.Tp
}

func (r *Confirm) GetSourceId() string {
	if r == nil {
		return ""
	}
	return r.SourceId
}

func (r *Confirm) GetDelay() string {
	if r == nil {
		return ""
	}
	return r.Delay
}

func (r *StopDelayReply) GetOk() bool {
	if r == nil {
		return false
	}
	return r.Ok
}

func (r *StopDelayReply) GetError() string {
	if r == nil {
		return ""
	}
	return r.Error
}

func (r *StopDelayReply) GetMessage() string {
	if r == nil {
		return ""
	}
	return r.Message
}
