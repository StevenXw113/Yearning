package login

import (
	"github.com/cookieY/yee"
	"net"
	"sync"
	"time"
)

const (
	// maxFailedAttempts 同一 IP + 用户名允许的连续失败次数
	maxFailedAttempts = 5
	// lockDuration 触发限制后的锁定时长
	lockDuration = 15 * time.Minute
	// attemptWindow 失败计数的有效窗口，超过后重新计数
	attemptWindow = 15 * time.Minute
)

type attempt struct {
	failed    int
	firstFail time.Time
	lockedAt  time.Time
}

var (
	attemptMu sync.Mutex
	attempts  = make(map[string]*attempt)
)

func init() {
	// 定期清理过期记录，避免长期运行下 map 无界增长
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			attemptMu.Lock()
			for k, v := range attempts {
				if time.Since(v.firstFail) > attemptWindow && time.Since(v.lockedAt) > lockDuration {
					delete(attempts, k)
				}
			}
			attemptMu.Unlock()
		}
	}()
}

// clientKey 以来源 IP + 用户名作为限流维度。
// 只使用 RemoteAddr（不可伪造），不信任 X-Forwarded-For。
func clientKey(c yee.Context, username string) string {
	host := c.Request().RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host + "|" + username
}

func isLocked(key string) bool {
	attemptMu.Lock()
	defer attemptMu.Unlock()
	a, ok := attempts[key]
	if !ok {
		return false
	}
	if !a.lockedAt.IsZero() {
		if time.Since(a.lockedAt) < lockDuration {
			return true
		}
		delete(attempts, key)
	}
	return false
}

func recordFailure(key string) {
	attemptMu.Lock()
	defer attemptMu.Unlock()
	a, ok := attempts[key]
	if !ok || time.Since(a.firstFail) > attemptWindow {
		attempts[key] = &attempt{failed: 1, firstFail: time.Now()}
		return
	}
	a.failed++
	if a.failed >= maxFailedAttempts {
		a.lockedAt = time.Now()
	}
}

func resetAttempts(key string) {
	attemptMu.Lock()
	defer attemptMu.Unlock()
	delete(attempts, key)
}
