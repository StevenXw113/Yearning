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

仓库里已有一份**完整可跑的示例**：`forbid_truncate.go`（禁止 TRUNCATE，默认不生效）。
新增规则照抄它即可，一共四处改动（只有"需要新开关"时才需要第 2、3、4 步）：

| # | 位置 | 改什么 |
| --- | --- | --- |
| 1 | `internal/customrules/<规则>.go` | 实现 `Name()` / `Check()`，在 `init()` 里 `Register(...)`；逻辑只读 `Context` 与 `Config` |
| 2 | `internal/customrules/rule.go` | 在 `Config` 里加开关字段（自研层不依赖上游 proto） |
| 3 | `internal/server/check.go` | `runCustomRules` 里把 `enginev1.AuditRole.GetXxx()` 映射进 `Config` |
| 4 | proto + 主程序 + 前端 | `engine.proto` 加字段 → `src/engine/engine.go` 与 `src/engine/convert.go` 透传 → `front/src/views/manager/rules/rules.ts` 加开关行 + `front/src/lang/{zh-cn,en-us}/rule/index.ts` 加文案 |

第 4 步最容易漏：**只加前端开关、忘了 `convert.go` 的映射**，页面勾了也传不到引擎，
且不会报错——只是规则永远不生效。开关的命名要与 Go 字段名完全一致（前端用它当 JSON key）。

第 1 步的骨架（`forbid_truncate.go` 的简化版）：

```go
package customrules

func init() { Register(myRule{}) }

type myRule struct{}

func (myRule) Name() string { return "my-rule" }

func (myRule) Check(cfg Config, c Context) []Finding {
	// 开关关闭时不生效：新增规则不能改变线上既有审核行为
	if !cfg.MySwitch {
		return nil
	}
	if strings.Contains(strings.ToLower(c.SQL), "forbidden") {
		return []Finding{{Title: "my-rule", Content: "命中说明", Level: LevelError}}
	}
	return nil
}
```

补测试（参考 `forbid_truncate_test.go`：命中 / 开关关闭不命中 / 同前缀不误伤），跑 `go test ./...`；
启用时在「设置 → 审核规则」里勾上对应开关保存即可。

## 如何删除一条自定义规则

先想清楚是"停用"还是"移除"：

| 目的 | 做法 | 影响 |
| --- | --- | --- |
| **停用**（推荐、可随时恢复） | 在「设置 → 审核规则」里取消勾选开关并保存，或把该规则的级别设为「观察」 | 立即生效，不用发版；规则集变更会记入「变更历史」，可一键回滚 |
| **移除**（彻底删代码） | 按下面的清单删干净 | 需要重新编译发布引擎 |

移除清单（漏一步会留下"幽灵开关"）：

1. 删掉规则文件（如 `forbid_truncate.go`）与它的测试
2. `rule.go` 的 `Config` 里删掉该规则的开关字段
3. `internal/server/check.go` 的 `runCustomRules` 里删掉对应的映射
4. 清掉一路透传的开关接线：`engine.proto` 字段 → `src/engine/engine.go` → `src/engine/convert.go`
   → `front/src/views/manager/rules/rules.ts` 里的开关行 → `front/src/lang/{zh-cn,en-us}/rule/index.ts` 文案
5. 既有规则集 JSON 里残留的开关键无害（引擎会忽略未知键），需要时可让用户重新保存一次规则集清掉

`internal/archguard` 会挡住第 1 步留下的悬挂引用：删了文件却留着 `Config` 字段/映射时编译即报错。

## 使用说明（启用与排查）

- **启用**：规则只有在开关打开时才生效（示例 `forbid_truncate` 默认关闭，所以加规则不会突然拦线上 SQL）。
  在「设置 → 审核规则」勾选后保存；建议先把该规则级别设为「观察」或「提示」跑一段时间，
  确认误报可控再改成「拦截」（级别见规则集页的「级别」列）。
- **生效范围**：规则集按数据源绑定（数据源管理里选规则集），多套规则集可以给不同数据源用不同强度。
- **验证**：提交一条会命中的 SQL（或直接调 `PUT /api/v2/fetch/test`），检测结果里会出现
  `[规则名] 命中说明`；自研规则的命中状态跟随级别（1 审核不通过 / 2 警告 / 3 观察）。
- **排查"勾了没反应"**：① 前端开关的 `name` 与 Go 字段名是否一致；② `convert.go` 有没有映射（最容易漏）；
  ③ `check.go` 的 `runCustomRules` 有没有把它放进 `Config`；④ 规则逻辑里是否真的读了该开关。
- **升级上游不受影响**：`scripts/sync-bytebase.sh` 只重建 `internal/bytebase`，本目录不会被覆盖。

## 约束（由 `internal/archguard` 自动保证）

- 本目录**不得** import `internal/bytebase`（同步来的上游类型会把你绑死在上游版本上）。
  需要的能力请在 `internal/mysqlparse`（适配层）里补，或只用 `Context` 提供的输入。
- `Config` 由自研层定义，不直接暴露上游 proto，避免上游 proto 变动传导到这里。
