# engine

Yearning 的独立数据库引擎适配服务，通过 **gRPC** 替代原 Juno `net/rpc`，首期支持 MySQL，
后续可扩展 PostgreSQL / SQL Server / Oracle 等。架构参考 Bytebase 的
Parser / Advisor / Policy / Advice 设计。

## 现状

基于 `github.com/bytebase/parser`（BSD-3，独立 ANTLR4 MySQL 解析器）实现 MySQL 能力：

- `EngineService.Check` ✅ SQL 拆分 + 语法校验 + 基础 DDL/DML 规则（DROP、无 WHERE 的 UPDATE/DELETE），返回逐条 `Record`
- `EngineService.Query` ✅ 查询 SQL 拆分 + 逐条校验，返回逐条 `Record`（含敏感字段词表）
- `EngineService.MergeAlterTables` ✅ 同表 `ALTER TABLE` 合并，跨表/非 ALTER 原样保留
- `EngineService.Exec` ✅ 连目标业务库拆分执行工单 SQL，统计影响行返回逐条明细（含事务与失败处理）
- `EngineService.StopDelay` ⏸ 延迟工单调度需接入 Yearning 元库连接与调度器，暂未接入

审核规则以 Yearning `AuditRole` 开关为语义，规则为引擎自研，未复制 Bytebase advisor 规则代码。
`Exec` 的逐条执行明细由 Yearning 主程序回写 `core_sql_records` 表。

## 目录

```
engine/
├── proto/engine/v1/engine.proto   # gRPC 契约
├── gen/engine/v1/                 # 生成的 Go 代码（enginev1）
├── cmd/engine/                    # 服务端入口
├── internal/server/               # gRPC 实现骨架
├── buf.gen.yaml / buf.yaml        # proto 生成配置
├── Dockerfile
└── README.md
```

## 生成代码（改 proto 后执行）

```bash
cd engine
buf generate        # 需 PATH 含 protoc-gen-go / protoc-gen-go-grpc
```

## 运行

```bash
go run ./cmd/engine -addr :13307
```

## 构建（amd64 / arm64）

```bash
GOOS=linux GOARCH=amd64 go build -o bin/engine-linux-amd64 ./cmd/engine
GOOS=linux GOARCH=arm64 go build -o bin/engine-linux-arm64 ./cmd/engine
```

## 接入 Yearning

Yearning 通过本地 `replace` 引用本 module：

```go
// Yearning go.mod
require engine v0.0.0
replace engine => ./engine
```

并将 `src/lib/calls` 与各调用点切换到 `enginev1.EngineServiceClient`。
