package calls

import (
	"Yearning-go/src/lib/rediscmd"
	"Yearning-go/src/model"

	"engine/api"
)

// DataSourceKind 将数据源 DBType 映射为引擎方言标识。
// Yearning 约定：0=mysql、1=postgres、2=mongodb、3=redis、4=doris。
// PostgreSQL 与 mysql 同是 SQL 方言、同走 Bytebase 上游规则；MongoDB 与 Redis 在引擎里
// 走本仓库自研的规则（上游 bytebase 不提供这两种 advisor）；Doris 是 MySQL 协议兼容
// 但建表 DDL 自有语法，走「MySQL 解析器 + 自研 dorischeck」的混合审核。
// 其余类型返回空串由引擎拒绝。
func DataSourceKind(dbType int) string {
	switch dbType {
	case 0:
		return "mysql"
	case model.DBTypePostgreSQL:
		return "postgres"
	case model.DBTypeMongoDB:
		return "mongodb"
	case model.DBTypeRedis:
		return "redis"
	case model.DBTypeDoris:
		return "doris"
	}
	return ""
}

// OrderToAPI 将本地工单转为引擎 proto 工单。
func OrderToAPI(o *model.CoreSqlOrder) *api.Order {
	if o == nil {
		return nil
	}
	return &api.Order{
		WorkId:      o.WorkId,
		Username:    o.Username,
		Status:      uint32(o.Status),
		Type:        int32(o.Type),
		Backup:      uint32(o.Backup),
		Idc:         o.IDC,
		Source:      o.Source,
		SourceId:    o.SourceId,
		DataBase:    o.DataBase,
		Table:       o.Table,
		Date:        o.Date,
		Sql:         o.SQL,
		Text:        o.Text,
		Assigned:    o.Assigned,
		Delay:       o.Delay,
		RealName:    o.RealName,
		ExecuteTime: o.ExecuteTime,
		CurrentStep: int32(o.CurrentStep),
		OscInfo:     o.OSCInfo,
		File:        o.File,
	}
}

// RedisAudit 填齐 Redis 审核需要的运行时事实，并返回键状态快照。
//
// 四件事必须一起给，缺一个就有规则静默失效：部署形态（拓扑 13 条）、
// 生产标记（ProductionWrite）、实例版本（CommandCompatibility）、
// 键快照（TypeMismatch / OverwriteExisting / RequireTtl 等）。
// 非 Redis 数据源一律空操作：ds 不动、快照返回 nil。
func RedisAudit(ds *api.DataSource, s *model.CoreDataSource, sql, schema string) map[string]api.KeyState {
	if ds == nil || s == nil || s.DBType != model.DBTypeRedis {
		return nil
	}
	ds.RedisMode = model.RedisDeploymentOf(s)
	ds.IsProduction = s.IsProduction
	ds.RedisVersion = model.RedisVersionOf(s)

	db, err := model.RedisParseDB(schema)
	if err != nil {
		db = 0 // 库名非法时按 db0 探测：审核仍要看键状态，库名合法性由 DbScope 规则判
	}
	return rediscmd.KeySnapshot(s, db, sql)
}
