package service

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/cookieY/yee/logger"
	_ "github.com/go-sql-driver/mysql"
)

// sqlDBType 是 *sql.DB 的别名（instanceLockDB 返回类型在实现文件里是 *sql.DB）。
type sqlDBType = sql.DB

// openPool 建立独立于 gorm 的原生连接池（锁探测需要异于持锁连接的连接）。
func openPool(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	pool, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := pool.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

// openPoolForProbe 是语义化别名：锁探测连接。
var openPoolForProbe = openPool

// TestAcquireInstanceLock 需要真实 MySQL（GET_LOCK 是 MySQL 会话级特性）：
// YEARNING_TEST_META_DSN 指向可写的元库，未设置时跳过保证 CI 无外部依赖。
// 验证核心语义：首次获取成功 → 异连接二次获取被拒 → 释放后可再抢。
func TestAcquireInstanceLock(t *testing.T) {
	dsn := os.Getenv("YEARING_TEST_META_DSN")
	if dsn == "" {
		t.Skip("未设置 YEARING_TEST_META_DSN，跳过集成用例")
	}
	logger.LogCreator(3)

	orig := instanceLockDB
	defer func() { instanceLockDB = orig }()

	openDB := func(t *testing.T, dsn string) func() error {
		t.Helper()
		pool := openPool(t, dsn)
		instanceLockDB = func() (*sqlDBType, error) { return pool, nil }
		return func() error { return pool.Close() }
	}

	cleanup := openDB(t, dsn)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := AcquireInstanceLock(ctx); err != nil {
		t.Fatalf("首次获取应成功: %v", err)
	}

	// 异连接验证锁被持有、且抢不走
	other := openPoolForProbe(t, dsn)
	defer other.Close()
	var held int
	if err := other.QueryRow("SELECT IS_USED_LOCK(?) IS NOT NULL", instanceLockName).Scan(&held); err != nil || held != 1 {
		t.Fatalf("锁应处于被持状态: held=%d err=%v", held, err)
	}
	var second int64
	if err := other.QueryRow("SELECT GET_LOCK(?, 0)", instanceLockName).Scan(&second); err != nil {
		t.Fatalf("probe GET_LOCK: %v", err)
	}
	if second == 1 {
		t.Fatal("第二连接不应能抢到锁（单实例语义的根）")
	}
	_, _ = other.Exec("SELECT RELEASE_LOCK(?)", instanceLockName) // 防御性释放（正常应为 0 行为）

	// 取消保活 → 连接关闭锁随之释放 → 可重新获取
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var free int64
		if err := other.QueryRow("SELECT IS_USED_LOCK(?) IS NULL", instanceLockName).Scan(&free); err == nil && free == 1 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	if err := AcquireInstanceLock(ctx2); err != nil {
		t.Fatalf("释放后应可重新获取: %v", err)
	}
	cancel2()
}
