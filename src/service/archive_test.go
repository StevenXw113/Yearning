package service

import (
	"os"
	"testing"
	"time"

	"Yearning-go/src/model"

	"github.com/cookieY/yee/logger"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TestArchiveOnce 集成用例（YEARING_TEST_META_DSN 指向可写 MySQL，缺省跳过）：
// 建齐 6+6 张表 → 植入终态/非终态工单与明细、过期查询日志 → 归档一轮 →
// 验证：终态工单连同明细迁走、非终态与未到期保留、重复归档幂等、查询日志迁走。
func TestArchiveOnce(t *testing.T) {
	dsn := os.Getenv("YEARING_TEST_META_DSN")
	if dsn == "" {
		t.Skip("未设置 YEARING_TEST_META_DSN，跳过集成用例")
	}
	logger.LogCreator(3)
	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接元库失败: %v", err)
	}
	db := gdb.Session(&gorm.Session{AllowGlobalUpdate: true})

	tables := []interface{}{
		&model.CoreSqlOrder{}, &model.CoreSqlRecord{}, &model.CoreRollback{},
		&model.CoreOrderComment{}, &model.CoreWorkflowDetail{},
		&model.CoreQueryRecord{},
		&model.CoreSqlOrderArchive{}, &model.CoreSqlRecordArchive{}, &model.CoreRollbackArchive{},
		&model.CoreOrderCommentArchive{}, &model.CoreWorkflowDetailArchive{},
		&model.CoreQueryRecordArchive{},
	}
	for _, tb := range tables {
		_ = db.Migrator().DropTable(tb)
		if err := db.AutoMigrate(tb); err != nil {
			t.Fatalf("migrate %T: %v", tb, err)
		}
	}
	defer func() {
		for _, tb := range tables {
			_ = db.Migrator().DropTable(tb)
		}
	}()

	old := time.Now().AddDate(0, 0, -400).Format("2006-01-02 15:04")
	recent := time.Now().AddDate(0, 0, -3).Format("2006-01-02 15:04")

	// 植入：W-OLD 终态(完成)带明细/回滚/流水；W-AUDIT 非终态(审核中)带明细；W-RECENT 终态但未到期
	db.Exec("INSERT INTO core_sql_orders (work_id,username,status,type,date,sql,`text`) VALUES ('W-OLD', 'u', 1, 1, ?, 'SELECT 1', 'old')", old)
	db.Exec("INSERT INTO core_sql_records (work_id,sql,state,affect_row,time,error) VALUES ('W-OLD', 'SELECT 1', '已执行', 1, ?, '')", old)
	db.Exec("INSERT INTO core_rollbacks (work_id,sql) VALUES ('W-OLD', 'ROLLBACK')")
	db.Exec("INSERT INTO core_workflow_details (work_id,username,time,action) VALUES ('W-OLD', 'u', ?, 'executed')", old)
	db.Exec("INSERT INTO core_sql_orders (work_id,username,status,type,date,sql,`text`) VALUES ('W-AUDIT', 'u', 2, 1, ?, 'SELECT 2', 'auditing')", old)
	db.Exec("INSERT INTO core_sql_records (work_id,sql,state,affect_row,time,error) VALUES ('W-AUDIT', 'SELECT 2', '已执行', 1, ?, '')", old)
	db.Exec("INSERT INTO core_sql_orders (work_id,username,status,type,date,sql,`text`) VALUES ('W-RECENT', 'u', 1, 1, ?, 'SELECT 3', 'recent')", recent)
	for i := 0; i < 3; i++ {
		db.Exec("INSERT INTO core_query_records (work_id,sql,ex_time,time,source,schema,audit) VALUES ('Q-OLD', 'SELECT * FROM t', 10, ?, 's', 'db', '')", old)
	}

	prevDays := model.C.General.ArchiveDays
	model.C.General.ArchiveDays = 180
	defer func() { model.C.General.ArchiveDays = prevDays }()

	orders, queryRecords := RunArchiveOnce()
	if orders != 1 {
		t.Fatalf("应归档 1 单（终态且到期），实际 %d", orders)
	}
	if queryRecords != 3 {
		t.Fatalf("应归档 3 行查询日志，实际 %d", queryRecords)
	}

	// 终态工单 + 明细全部迁走
	assertTableCount(t, db, "core_sql_orders", "work_id='W-OLD'", 0)
	assertTableCount(t, db, "core_sql_records", "work_id='W-OLD'", 0)
	assertTableCount(t, db, "core_rollbacks", "work_id='W-OLD'", 0)
	assertTableCount(t, db, "core_workflow_details", "work_id='W-OLD'", 0)
	assertTableCount(t, db, "core_sql_orders_archive", "work_id='W-OLD'", 1)
	assertTableCount(t, db, "core_sql_records_archive", "work_id='W-OLD'", 1)
	assertTableCount(t, db, "core_rollbacks_archive", "work_id='W-OLD'", 1)
	// 非终态（审核中）与未到期工单必须保留
	assertTableCount(t, db, "core_sql_orders", "work_id='W-AUDIT'", 1)
	assertTableCount(t, db, "core_sql_records", "work_id='W-AUDIT'", 1)
	assertTableCount(t, db, "core_sql_orders", "work_id='W-RECENT'", 1)
	// 查询日志迁走
	assertTableCount(t, db, "core_query_records", "1=1", 0)
	assertTableCount(t, db, "core_query_records_archive", "1=1", 3)

	// 幂等：重跑一轮，无迁移、无报错、无重复行
	orders2, q2 := RunArchiveOnce()
	if orders2 != 0 || q2 != 0 {
		t.Fatalf("重跑应无迁移: orders=%d q=%d", orders2, q2)
	}
	assertTableCount(t, db, "core_sql_orders_archive", "work_id='W-OLD'", 1)
	assertTableCount(t, db, "core_query_records_archive", "1=1", 3)
}

func assertTableCount(t *testing.T, db *gorm.DB, table, where string, want int64) {
	t.Helper()
	var n int64
	if err := db.Table(table).Where(where).Count(&n).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if n != want {
		t.Fatalf("%s WHERE %s = %d, want %d", table, where, n, want)
	}
}
