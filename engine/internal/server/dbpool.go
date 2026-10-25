package server

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// execPools 按连接串缓存执行池:每 Exec 新建即弃会让同一数据源
// 在批次执行/连续工单下反复支付 TCP+auth 握手。
//
// ponytail: 无淘汰,条目=数据源×库 的量级(几十),口令变更走新 DSN
// 自然分流,旧条目仅剩池对象本身不占连接(ConnMaxLifetime 已封顶)。
// 条目失控时再上 LRU。
var execPools sync.Map

func execPool(driver, dsn string, opts ...func(*sql.DB)) (*sql.DB, error) {
	key := driver + "|" + dsn
	if v, ok := execPools.Load(key); ok {
		return v.(*sql.DB), nil
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	for _, opt := range opts {
		opt(db)
	}
	db.SetConnMaxIdleTime(time.Minute)
	db.SetConnMaxLifetime(time.Minute * 3)
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	actual, loaded := execPools.LoadOrStore(key, db)
	if loaded {
		// 并发下另一个 goroutine 先存了:用它的,关掉自己这份
		db.Close()
		return actual.(*sql.DB), nil
	}
	return db, nil
}
