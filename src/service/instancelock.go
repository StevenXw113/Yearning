package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"Yearning-go/src/model"

	"github.com/cookieY/yee/logger"
)

// instanceLockName MySQL 命名锁名：同一元数据库只允许一个 Yearning 主实例。
// cron 重复统计（TotalTickets 重复插入）、OSC 中止（oscRunning 是进程内表）、
// 执行中超时自愈等均依赖单实例；多实例会互相破坏。
const instanceLockName = "yearning_primary_instance"

// instanceLockPing 持锁连接的保活间隔：既防服务器 wait_timeout 掐断空闲连接，
// 又把「连接断开 → 锁释放 → 他人接管」的暴露窗口压在数十秒内。
const instanceLockPing = 15 * time.Second

// AcquireInstanceLock 获取单实例互斥锁并启动保活。返回错误表示已有实例在运行。
//
// 实现用 MySQL 的会话级命名锁 GET_LOCK：从连接池钉出一条专用连接持锁，
// 连接断开（网络抖动、实例崩溃）锁即自动释放——这是它优于自建锁表的关键：
// 崩溃的实例不会留死锁。代价是保活失败后必须**重连重抢**，抢不到说明锁已被
// 其他实例接管，本实例继续跑会破坏单实例假设，只能退出进程。
func AcquireInstanceLock(ctx context.Context) error {
	conn, err := grabLock(ctx)
	if err != nil {
		return err
	}
	go keepInstanceLock(ctx, conn)
	return nil
}

// instanceLockDB 返回元数据库连接池。独立出来是为了测试可注入假池
// （真实环境即 model.DB() 底层的 *sql.DB）。
var instanceLockDB = func() (*sql.DB, error) {
	return model.DB().DB()
}

// grabLock 在（新）连接上抢锁；GET_LOCK(name, 0) 立即返回：1=抢到 0=被占。
func grabLock(ctx context.Context) (*sql.Conn, error) {
	pool, err := instanceLockDB()
	if err != nil {
		return nil, fmt.Errorf("获取元数据库连接池失败: %w", err)
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("建立持锁连接失败: %w", err)
	}
	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", instanceLockName).Scan(&got); err != nil || !got.Valid || got.Int64 != 1 {
		_ = conn.Close()
		return nil, fmt.Errorf("另一 Yearning 实例正在运行（GET_LOCK=%d err=%v），本实例拒绝启动", got.Int64, err)
	}
	return conn, nil
}

// keepInstanceLock 持锁保活：连接断了就重连重抢，重抢失败（锁被接管）必须退出。
func keepInstanceLock(ctx context.Context, conn *sql.Conn) {
	ticker := time.NewTicker(instanceLockPing)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = conn.Close()
			return
		case <-ticker.C:
			if pingOK(ctx, conn) {
				continue
			}
			_ = conn.Close()
			newConn, err := grabLock(ctx)
			if err != nil {
				// 锁已被其他实例接管：本实例的 cron/OSC/自愈再跑就会双写，
				// 唯一安全的动作是退出（由容器编排拉起后重新排队抢锁）
				logger.DefaultLogger.Errorf("单实例锁丢失且重抢失败，本实例退出: %v", err)
				time.Sleep(3 * time.Second) // 留给日志落盘
				panic(err)                  // os.Exit 跳过 defer；panic 让进程带非零退出码结束
			}
			logger.DefaultLogger.Warn("单实例锁连接中断，已重连并重新持锁")
			conn = newConn
		}
	}
}

func pingOK(ctx context.Context, conn *sql.Conn) bool {
	var one int
	err := conn.QueryRowContext(ctx, "SELECT 1").Scan(&one)
	// 元数据库瞬时报错不等同于锁丢失（可能只是这次查询超时）：只有确认换连接
	// 才作数，grabLock 失败与否才是最终裁决
	return err == nil
}
