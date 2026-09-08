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
	}
}

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
	}
}
