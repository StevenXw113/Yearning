# 实施计划:gRPC → HTTP+JSON

> 设计依据:docs/design-grpc-to-http.md(已含实现修正:protobuf 依赖因 bytebase 内部结构保留;api 包补 GetError() 让调用面零改动)

## 任务分解(顺序执行,每步可独立验证)

### T1 engine/api 契约包(TDD 先行)
- `engine/api/api.go`:手写 struct,字段名与 proto 一致(snake_case json tag);DataSource/AuditRole/Order/Record/Confirm + 5 组 Request/Reply;Reply 与 Record 带 `GetError() string` 一行方法(兼容调用面)。
- `engine/api/api_test.go`:JSON 往返测试(锁定字段名/类型;重点:AuditRole 94 字段、rule_levels map、Record 回滚字段)。
- 验证:`go test ./api/`(engine 模块根)。

### T2 engine 服务端 HTTP 化(TDD)
- `engine/internal/server/http.go`:stdlib mux + 三个中间件(auth 常数时间比较/埋点/MaxBytesReader 64MB)+ `/healthz`;业务错误形态:200 + rep(Ok/Error);401 未授权;400 解码失败;500 handler error。
- `engine/internal/server/http_test.go`:先写失败测试——auth 启用/禁用/错误令牌、healthz 免认证、Check 端点业务错误、消息上限、metrics 计数。
- 各 impl 文件(check/exec/query/merge/rules/...)与对应测试:`enginev1` → `api` 机械替换(测试直调方法,不走传输,应全绿)。
- 删除 interceptor.go、stats.go(逻辑迁入 http.go);interceptor_test.go 的 auth 用例迁入 http_test.go。
- main.go:grpc.NewServer → http.Server(复用 LoadServerTLS、emetrics、优雅 Shutdown;删 health/reflection 注册)。
- 验证:`go build ./... && go test ./...`(engine 模块)。

### T3 删除 gRPC 工具链(engine)
- 删 `engine/proto/`、`engine/gen/`、`engine/buf.yaml`、`engine/buf.gen.yaml`。
- engine/go.mod:`go mod tidy` 移除 grpc(genproto 等),**保留 protobuf**(bytebase 内部结构依赖)。
- 验证:`go build ./... && go vet ./... && go test ./...`;`grep -r grpc --include=*.go` 无业务残留(qprobe/emetrics 除外——本来无关)。

### T4 主服务客户端(src/lib/calls)
- `rpc.go` → http.Client 单例:TLS Transport(复用 commonTLSConfig)、Bearer 头、JSON 编解码。
- 重试:仅幂等方法(Check/Query/MergeAlterTables)连接失败/502/503/504 时 ≤5 次指数退避(0.3s→2s);**Exec/StopDelay 单次**(修 P1)。
- `health.go`:埋点与健康翻转逻辑原样保留,判定改 net 错误/HTTP 状态码;`err.go`、`tls.go` 基本不动;convert.go 目标类型换 api。
- `calls/client_test.go`:幂等方法重试、Exec 不重试、健康翻转告警(httptest.Server)。
- 验证:`go build ./src/lib/calls/ && go test ./src/lib/calls/`。

### T5 调用点与 checkdiff 适配
- 8 个调用点文件 + src/engine/convert.go + 两个 convert_test:`engine/gen/engine/v1` → `engine/api`,`enginev1.` → `api.`;`ToProto/FromProto` 函数名 → `ToAPI/FromAPI`。
- tools/checkdiff:dial gRPC → HTTP client,对比语义不变。
- 验证:`go build ./...`(主模块)。

### T6 主模块收尾
- go.mod tidy 去 grpc;`go vet ./...`;全量 `go test ./...`。
- 注释/文档:src/model/global.go、conf 配置注释、engine/README.md(grpcurl→curl 示例)、docs/DEVELOPMENT.md、docs/design-grpc-to-http.md 状态更新。
- 验证:全量测试绿 + grep 无 gRPC 残留文档误导。

## 风险与回退
- 单分支原子提交序列,任一阶段失败 `git restore` 回退。
- 前端零涉及;部署需主服务与引擎同批升级(协议不兼容,设计已注明)。
