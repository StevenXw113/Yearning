# internal/customrules

团队自研的审核规则。与从 Bytebase 同步的 `internal/bytebase/` **物理隔离**：
`scripts/sync-bytebase.sh` 只重建 `internal/bytebase`，**不会触碰本目录**，
因此升级上游规则时这里的自定义规则不会被覆盖或冲掉。

## 两层规则

| 层 | 位置 | 来源 | 同步时 |
| --- | --- | --- | --- |
| 上游规则 | `internal/bytebase/plugin/advisor/mysql` | Bytebase 官方仓库 | **整体重建**（勿手改） |
| 自研规则 | `internal/customrules`（本目录） | 自己写 | **完全不动** |

两层由 `internal/server/check.go` 的 `Check` 编排在同一链路里，命中都映射为统一的 `Record`：

```
单条 SQL
  → customrules.All() 逐条检查（自研，先跑，命中即短路）
  → review() → Bytebase advisor.Check（上游，按审核规则枚举）
  → enginev1.Record
```

## 如何新增一条规则

1. 在 `internal/customrules/` 下新建文件（例如 `no_truncate.go`）：

```go
package customrules

func init() { Register(noTruncate{}) }

type noTruncate struct{}

func (noTruncate) Name() string { return "no-truncate" }

func (noTruncate) Check(cfg Config, c Context) []Finding {
	// c.SQL / c.Schema 为输入；只读，不要调用外部服务
	if cfg.AllowTruncate {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(c.SQL)), "truncate ") {
		return []Finding{{Title: "no-truncate", Content: "禁止执行 TRUNCATE 操作", Level: LevelError}}
	}
	return nil
}
```

2. 若规则需要新的开关，在 `rule.go` 的 `Config` 中加字段，并在
   `internal/server/check.go` 的 `runCustomRules` 里从 `enginev1.AuditRole` 映射过来。

3. 补测试（参考 `drop_test.go`），运行 `go test ./...`。

## 约束（由 `internal/archguard` 自动保证）

- 本目录**不得** import `internal/bytebase`（同步来的上游类型会把你绑死在上游版本上）。
  需要的能力请在 `internal/mysqlparse`（适配层）里补，或只用 `Context` 提供的输入。
- `Config` 由自研层定义，不直接暴露上游 proto，避免上游 proto 变动传导到这里。
