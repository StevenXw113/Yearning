# 设计:引擎通信协议 gRPC → HTTP+JSON

> 状态:**已实施完成**(2026-10-25,契约包/HTTP 服务端/客户端/工具/文档全部落地,两模块 build+vet+test 全绿;实施记录见 docs/plan-grpc-to-http.md)
> 日期:2026-10-24
> 决策背景:用户拍板弃用 gRPC 改 HTTP+JSON。取舍分析见会话记录:进程边界保留,仅换传输/序列化层。

## 1. 目标与非目标

**目标**
- 引擎(main↔engine)通信改用 stdlib `net/http` + `encoding/json`,删除 grpc/protobuf/buf 全套工具链依赖。
- 主服务调用点、服务端各方言实现、测试的行为语义不变(契约字段一比一平移)。
- 顺手修复既有 P1 风险:非幂等的 Exec 不再被自动重试。

**非目标**
- 不动业务逻辑(审核/执行/回滚/OSC 全部原样)。
- 不引入任何新依赖、新框架(engine 侧用 stdlib mux,与既有 /metrics 端点同模式)。
- 不做协议双栈/灰度共存(engine 与主服务同仓库同版本发布,一起滚动)。

## 2. 契约层:engine/api 包

新建 `engine/api/api.go`,手写 plain struct,字段名与 proto 完全同名(snake_case json tag),类型名与 proto 生成类型同名:

- 请求/响应:`CheckRequest/CheckReply`、`MergeAlterTablesRequest/Reply`、`ExecRequest/ExecReply`、`QueryRequest/QueryReply`、`StopDelayRequest/Reply`
- 载荷:`DataSource`、`AuditRole`(94 字段,含 rule_levels map)、`Order`、`Record`、`Confirm`

**删除**:`engine/proto/`、`engine/gen/`、`engine/buf.yaml`、`engine/buf.gen.yaml`;两个 go.mod 移除 `google.golang.org/grpc`、`google.golang.org/protobuf`。

要点:engine 模块不能 import 主模块(反向依赖),所以转换层(`src/engine/convert.go`、`src/lib/calls/convert.go`)保留,只把目标类型从 enginev1 换成 api,字段映射逻辑不变。

## 3. 服务端(engine/internal/server)

`grpc.NewServer` → stdlib `http.ServeMux`:

| 路由 | 方法 | 替代 |
|---|---|---|
| /v1/check | POST | Check |
| /v1/merge-alter-tables | POST | MergeAlterTables |
| /v1/exec | POST | Exec |
| /v1/query | POST | Query |
| /v1/stop-delay | POST | StopDelay |
| /healthz | GET | grpc_health_v1(探针/编排就绪检测) |
| /metrics | GET | 已有,不动 |

- **认证**:Bearer 令牌中间件,常数时间比较,豁免 /healthz(语义与现 interceptor.go 一致)。
- **埋点**:统一中间件计时(替代 UnaryStats/StreamStats),emetrics 复用。
- **消息上限**:`http.MaxBytesReader` 64MB。
- **TLS/mTLS**:http.Server TLSConfig,复用现有 LoadServerTLS。
- **优雅退出**:`Server.Shutdown(ctx)` 替代 GracefulStop。
- **删除**:engine.go 的 UnimplementedEngineServiceServer 嵌入、interceptor.go、stats.go、health/reflection 注册。
- 业务错误形态不变:reply.Ok=false + Error 承载;HTTP 状态码只表达传输/协议层(401 未授权、500 内部错误)。

## 4. 客户端(src/lib/calls)

保留 `calls.Client()` 进程级单例,内部从 grpc.ClientConn 换成 http.Client:

- Transport TLS + Bearer 请求头注入,配置字段 RpcAddr/RpcToken/RpcTLSCA/Cert/Key/ServerName 全部沿用(仅注释 gRPC→HTTP)。
- 方法名、参数、返回值形态与现状一致(类型前缀 enginev1→api),**8 个调用点 + 11 个文件只改 import 与类型前缀**。
- **重试语义(修 P1)**:幂等方法 Check/Query/MergeAlterTables 保留简单重试循环(连接失败/503,≤5 次指数退避 0.3s→2s);**Exec 与 StopDelay 不自动重试**——引擎执行到一半挂掉时重发会造成已生效语句二次执行。
- 健康翻转告警(engineHealthFlipped)、EngineRPCDuration/EngineRPCTotal/EngineUnavailableTotal/EngineHealthy 埋点逻辑原样保留,判定条件从 gRPC status code 改为 net 错误/HTTP 状态码。

## 5. 周边适配

- `tools/checkdiff`(新旧引擎审核结果对比工具):改用 HTTP dial,对比语义不变。
- `src/model/global.go` 注释更新;"引擎 gRPC 监听地址"→"引擎 HTTP 监听地址"。
- 部署:engine 容器启动参数 -addr/-auth-token/-tls-* 不变;主服务与引擎需同版本一起升级(跨版本协议不再兼容,与现 gRPC 版本锁定语义一致)。
- 调试:grpcurl → curl,更直观。

## 6. 测试(TDD 顺序)

1. 先写 engine/api 的 JSON 往返测试(契约锁定:字段名/类型不漂移)。
2. Red:新 HTTP 端点测试(httptest,复用现有 check/exec/... 测试用例的断言逻辑,前缀改名)。
3. Green:实现服务端 mux + 中间件。
4. 客户端:重试语义测试(幂等方法重试、Exec 不重试、健康翻转告警)。
5. interceptor_test/stats 相关测试改写或删除;15 个测试文件前缀机械替换。

## 7. 刻意简化(ponytail 标注)

- **不留 proto 文件**:契约即 Go struct + 注释。未来若有 Go 之外的客户端(Python 调引擎)需手写 JSON——出现再说。
- **JSON 序列化性能**:64MB 大工单 JSON 编解码慢于 protobuf 且内存峰值更高;工单执行低频,可接受。基准证明是瓶颈再考虑流式或分片。
- **无协议版本号**:URL 带 /v1 即版本,加字段向前兼容(JSON 缺省零值),语义变更才开 /v2。
