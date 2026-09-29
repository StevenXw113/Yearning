package engine

import (
	enginev1 "engine/gen/engine/v1"
)

// AuditRoleToProto 将本地审核规则转为引擎 proto 规则。
func AuditRoleToProto(r *AuditRole) *enginev1.AuditRole {
	if r == nil {
		return nil
	}
	return &enginev1.AuditRole{
		DmlTransaction:                 r.DMLTransaction,
		DmlAllowLimitStmt:              r.DMLAllowLimitSTMT,
		DmlInsertColumns:               r.DMLInsertColumns,
		DmlMaxInsertRows:               int32(r.DMLMaxInsertRows),
		DmlWhere:                       r.DMLWhere,
		DmlWhereExprValueIsNull:        r.DMLWhereExprValueIsNull,
		DmlOrder:                       r.DMLOrder,
		DmlSelect:                      r.DMLSelect,
		DmlAllowInsertNull:             r.DMLAllowInsertNull,
		DmlInsertMustExplicitly:        r.DMLInsertMustExplicitly,
		DdlEnablePrimaryKey:            r.DDLEnablePrimaryKey,
		DdlCheckTableComment:           r.DDLCheckTableComment,
		DdlCheckColumnComment:          r.DDlCheckColumnComment,
		DdlCheckColumnNullable:         r.DDLCheckColumnNullable,
		DdlCheckColumnDefault:          r.DDLCheckColumnDefault,
		DdlEnableAcrossDbRename:        r.DDLEnableAcrossDBRename,
		DdlEnableAutoincrementInit:     r.DDLEnableAutoincrementInit,
		DdlEnableAutoIncrement:         r.DDLEnableAutoIncrement,
		DdlEnableAutoincrementUnsigned: r.DDLEnableAutoincrementUnsigned,
		DdlEnableDropTable:             r.DDLEnableDropTable,
		DdlEnableDropDatabase:          r.DDLEnableDropDatabase,
		DdlEnableNullIndexName:         r.DDLEnableNullIndexName,
		DdlIndexNameSpec:               r.DDLIndexNameSpec,
		DdlMaxKeyParts:                 uint32(r.DDLMaxKeyParts),
		DdlMaxKey:                      uint32(r.DDLMaxKey),
		DdlMaxCharLength:               uint32(r.DDLMaxCharLength),
		MaxTableNameLen:                int32(r.MaxTableNameLen),
		MaxAffectRows:                  uint32(r.MaxAffectRows),
		MaxDdlAffectRows:               uint32(r.MaxDDLAffectRows),
		SupportCharset:                 r.SupportCharset,
		SupportCollation:               r.SupportCollation,
		CheckIdentifier:                r.CheckIdentifier,
		MustHaveColumns:                r.MustHaveColumns,
		DdlMultiToCommit:               r.DDLMultiToCommit,
		DdlPrimaryKeyMust:              r.DDLPrimaryKeyMust,
		DdlAllowColumnType:             r.DDLAllowColumnType,
		DdlImplicitTypeConversion:      r.DDLImplicitTypeConversion,
		DdlAllowPriNotInt:              r.DDLAllowPRINotInt,
		DdlAllowMultiAlter:             r.DDLAllowMultiAlter,
		DdlEnableForeignKey:            r.DDLEnableForeignKey,
		DdlTablePrefix:                 r.DDLTablePrefix,
		DdlColumnsMustHaveIndex:        r.DDLColumnsMustHaveIndex,
		DdlAllowChangeColumnPosition:   r.DDLAllowChangeColumnPosition,
		DdlCheckFloatDouble:            r.DDLCheckFloatDouble,
		IsOsc:                          r.IsOSC,
		OscExpr:                        r.OSCExpr,
		OscSize:                        uint32(r.OscSize),
		AllowCreateView:                r.AllowCreateView,
		AllowCreateViewWithSelectStar:  r.AllowCrateViewWithSelectStar,
		AllowCreatePartition:           r.AllowCreatePartition,
		AllowSpecialType:               r.AllowSpecialType,
		PriRollBack:                    r.PRIRollBack,
		RuleLevels:                     r.RuleLevel,
		DdlForbidTruncate:              r.DDLForbidTruncate,
		MongoForbidEmptyFilter:         r.MongoForbidEmptyFilter,
		MongoForbidDangerous:           r.MongoForbidDangerous,
		MongoForbidWhere:               r.MongoForbidWhere,
		MongoForbidDropCollection:      r.MongoForbidDropCollection,
		MongoForbidSystemCollection:    r.MongoForbidSystemCollection,
		MongoForbidAdminCommand:        r.MongoForbidAdminCommand,
		MongoForbidImmutableId:         r.MongoForbidImmutableID,
		MongoForbidCappedConvert:       r.MongoForbidCappedConvert,
		MongoForbidCollMod:             r.MongoForbidCollMod,
		MongoIndexKeyLimit:             int32(r.MongoIndexKeyLimit),
		MongoIndexNameSpec:             r.MongoIndexNameSpec,
		MongoCollectionPrefix:          r.MongoCollectionPrefix,
		MongoMaxCollectionNameLen:      int32(r.MongoMaxCollectionNameLen),
		MongoRegexUnanchored:           r.MongoRegexUnanchored,
		MongoNegationOperator:          r.MongoNegationOperator,
		MongoOrClause:                  r.MongoOrClause,
		MongoLargeInList:               int32(r.MongoLargeInList),
		MongoQueryForbidNoFilter:       r.MongoQueryForbidNoFilter,
		MongoQueryForbidLookup:         r.MongoQueryForbidLookup,
		MongoQueryForbidNoLimit:        r.MongoQueryForbidNoLimit,
		MongoMaxAffectRows:             int32(r.MongoMaxAffectRows),
	}
}

// 审核模式：引擎的 Check 服务两种入口，必须由调用方明说在审哪一种。
// 与 engine/internal/mongocheck 的 ModeWrite / ModeQuery 常量一一对应——
// 主程序不能 import engine 的内部包，这里是两边唯一的契约点，改一处要同步另一处。
const (
	// CheckModeWrite 变更命令审核：申请页检测、工单执行前复检
	CheckModeWrite = "write"
	// CheckModeQuery 只读命令审核：查询页
	CheckModeQuery = "query"
)

// RecordFromProto 将引擎审核记录转为本地 engine.Record。
func RecordFromProto(p *enginev1.Record) Record {
	if p == nil {
		return Record{}
	}
	return Record{
		SQL:              p.Sql,
		AffectRows:       uint(p.AffectRows),
		Status:           p.Status,
		Error:            p.Error,
		Level:            uint8(p.Level),
		ExecTime:         p.ExecTime,
		Table:            p.Table,
		Schema:           p.Schema,
		IsOSC:            p.IsOsc,
		InsulateWordList: p.InsulateWordList,
		RollBack:         p.Rollback,
	}
}
