package model

// 历史表归档：完结工单及其明细、过期查询日志，迁移到同构 *_archive 表。
//
// 设计要点：
//   - 归档表与原表**完全同构**（含自增主键），迁移用 INSERT INTO ... SELECT *，
//     work_id 重复归档会因主键冲突失败——与「同事务内删除」配合天然幂等；
//   - 只在独立事务里逐单迁移（orders + 全部明细同进同出），归档中途崩溃不会
//     出现「工单没了、明细还在」的撕裂；
//   - 列表查询不加任何条件：行被移走后自动从列表消失，审计完整性由归档
//     查询接口（record 包）兜底——数据还在，只是换了个地方。

// CoreSqlOrderArchive 归档工单主表（core_sql_orders_archive）。
type CoreSqlOrderArchive struct {
	CoreSqlOrder
}

func (CoreSqlOrderArchive) TableName() string { return "core_sql_orders_archive" }

// CoreSqlRecordArchive 归档执行明细（core_sql_records_archive）。
type CoreSqlRecordArchive struct {
	CoreSqlRecord
}

func (CoreSqlRecordArchive) TableName() string { return "core_sql_records_archive" }

// CoreRollbackArchive 归档回滚语句（core_rollbacks_archive）。
type CoreRollbackArchive struct {
	CoreRollback
}

func (CoreRollbackArchive) TableName() string { return "core_rollbacks_archive" }

// CoreOrderCommentArchive 归档工单评论（core_order_comments_archive）。
type CoreOrderCommentArchive struct {
	CoreOrderComment
}

func (CoreOrderCommentArchive) TableName() string { return "core_order_comments_archive" }

// CoreWorkflowDetailArchive 归档流程流水（core_workflow_details_archive）。
type CoreWorkflowDetailArchive struct {
	CoreWorkflowDetail
}

func (CoreWorkflowDetailArchive) TableName() string { return "core_workflow_details_archive" }

// CoreQueryRecordArchive 归档查询审计日志（core_query_records_archive）。
// 查询日志是全库增长最快的表（每条查询一行、含 SQL 全文）。
type CoreQueryRecordArchive struct {
	CoreQueryRecord
}

func (CoreQueryRecordArchive) TableName() string { return "core_query_records_archive" }
