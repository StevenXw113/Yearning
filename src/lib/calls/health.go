package calls

import (
	"context"
	"sync/atomic"
	"time"

	"Yearning-go/src/lib/metrics"
	"Yearning-go/src/lib/pusher"

	enginev1 "engine/gen/engine/v1"

	"github.com/cookieY/yee/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// engineDown 引擎当前是否处于「不可用」状态（用于健康翻转告警：只在
// 健康→异常、异常→恢复各通知一次，防止每次请求都告警形成风暴）。
var engineDown atomic.Bool

// instrumented 包装 EngineServiceClient：所有 RPC 统一埋点（耗时/成败/健康
// 状态/不可用告警），调用点零改动。
type instrumented struct {
	enginev1.EngineServiceClient
}

func (c instrumented) Check(ctx context.Context, in *enginev1.CheckRequest, opts ...grpc.CallOption) (*enginev1.CheckReply, error) {
	return do(ctx, "Check", opts, func(o []grpc.CallOption) (*enginev1.CheckReply, error) {
		return c.EngineServiceClient.Check(ctx, in, o...)
	})
}

func (c instrumented) Exec(ctx context.Context, in *enginev1.ExecRequest, opts ...grpc.CallOption) (*enginev1.ExecReply, error) {
	return do(ctx, "Exec", opts, func(o []grpc.CallOption) (*enginev1.ExecReply, error) {
		return c.EngineServiceClient.Exec(ctx, in, o...)
	})
}

func (c instrumented) Query(ctx context.Context, in *enginev1.QueryRequest, opts ...grpc.CallOption) (*enginev1.QueryReply, error) {
	return do(ctx, "Query", opts, func(o []grpc.CallOption) (*enginev1.QueryReply, error) {
		return c.EngineServiceClient.Query(ctx, in, o...)
	})
}

func (c instrumented) MergeAlterTables(ctx context.Context, in *enginev1.MergeAlterTablesRequest, opts ...grpc.CallOption) (*enginev1.MergeAlterTablesReply, error) {
	return do(ctx, "MergeAlterTables", opts, func(o []grpc.CallOption) (*enginev1.MergeAlterTablesReply, error) {
		return c.EngineServiceClient.MergeAlterTables(ctx, in, o...)
	})
}

func (c instrumented) StopDelay(ctx context.Context, in *enginev1.StopDelayRequest, opts ...grpc.CallOption) (*enginev1.StopDelayReply, error) {
	return do(ctx, "StopDelay", opts, func(o []grpc.CallOption) (*enginev1.StopDelayReply, error) {
		return c.EngineServiceClient.StopDelay(ctx, in, o...)
	})
}

// rpcReply 各 Reply 的公共形态（GetError 由 proto 生成）。
type rpcReply interface{ GetError() string }

// do 统一计时与成败归因。业务错误（引擎返回 Ok=false / Error 非空）不算
// 「不可用」——那是审核/执行结果；只有传输层失败（连接失败、UNAVAILABLE、
// DeadlineExceeded、认证不匹配等）才计为引擎异常并告警。
func do[T rpcReply](ctx context.Context, method string, opts []grpc.CallOption, call func([]grpc.CallOption) (T, error)) (T, error) {
	start := time.Now()
	rep, err := call(opts)
	metrics.EngineRPCDuration.Observe(time.Since(start))

	if err != nil && isTransportFailure(err) {
		metrics.EngineUnavailableTotal.Inc()
		metrics.EngineRPCTotal.With(method, "error").Inc()
		engineHealthFlipped(false, err.Error())
		return rep, err
	}
	metrics.EngineRPCTotal.With(method, "ok").Inc()
	engineHealthFlipped(true, "")
	return rep, err
}

// isTransportFailure 判定是否传输层失败（而非业务拒绝）。
func isTransportFailure(err error) bool {
	if err == nil {
		return false
	}
	switch status.Code(err) {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Canceled, codes.Unauthenticated, codes.Unimplemented:
		// Unavailable=引擎挂了/重启窗口；Unauthenticated/Unimplemented=两端版本或
		// 令牌配置不匹配，同样是「引擎不可用」需要人看到
		return true
	}
	return false
}

// engineHealthFlipped 健康状态翻转处理：变化时更新 Gauge 并告警一次。
func engineHealthFlipped(healthy bool, reason string) {
	if healthy {
		metrics.EngineHealthy.Store(1)
		if engineDown.CompareAndSwap(true, false) {
			logger.DefaultLogger.Info("审核引擎恢复可用")
			pusher.SystemAlert("审核引擎恢复", "引擎 gRPC 通道已恢复可用。")
		}
		return
	}
	metrics.EngineHealthy.Store(0)
	if engineDown.CompareAndSwap(false, true) {
		logger.DefaultLogger.Errorf("审核引擎不可用: %s", reason)
		pusher.SystemAlert("审核引擎不可用", "引擎 gRPC 调用失败："+reason+"。工单执行/SQL 检测将失败，请检查引擎容器状态。")
	}
}

// IsEngineHealthy 引擎最近一次 RPC 是否成功（/health 用）。
func IsEngineHealthy() bool { return engineDown.Load() == false }
