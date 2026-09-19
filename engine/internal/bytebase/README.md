# internal/bytebase

本目录内化（vendor-in）了 **Bytebase 的 SQL 解析器与 MySQL 审核规则**，供 engine 的审核链路直接调用。
engine 自身只做「拆分 → 逐条审核 → 结果映射」，不重复实现任何规则，也不依赖第三方封装 module。

- 上游仓库：`https://github.com/bytebase/bytebase`
- 当前同步版本：见 [`UPSTREAM`](./UPSTREAM)（默认取最新 release tag）
- 许可：MIT Expat，见 [`LICENSE`](./LICENSE)（上游 `enterprise` 相关目录未纳入）
- 目录布局**镜像上游 `backend/`**，便于对照升级：

```
engine/internal/bytebase/          # 对应 bytebase/backend/
├── plugin/advisor/                # 审核规则框架（advisor.go、builtin_rules.go、code）
│   ├── sql_review.go              # 仅保留规则类型常量与模板解析（已砍 SQLReviewCheck）
│   └── mysql/                     # MySQL 全部审核规则实现
├── plugin/parser/base/            # 解析器接口、AST、拆分、错误类型
├── plugin/parser/mysql/           # MySQL 解析 / 拆分 / 资源变更
├── plugin/parser/tokenizer/       # 通用分词器
├── common/                        # 审核链路用到的通用工具（已裁剪）
├── store/model/                   # 元数据模型
├── generated-go/store/            # protobuf 生成代码
├── UPSTREAM                       # 同步来源与版本（脚本自动生成）
├── README.md
└── LICENSE
```

## 与上游的差异（裁剪清单）

每一处裁剪都写在 [`../../scripts/trim-bytebase.py`](../../scripts/trim-bytebase.py) 中，且**必须命中**，
否则同步脚本报错退出——说明上游结构变了，需要人工调整脚本，而不是静默产出一份编译不过的内化代码。

主要裁剪：

1. **只保留 MySQL**：删除 `plugin/advisor/{mssql,oceanbase,oracle,pg,redshift,snowflake,tidb}`。
2. **砍掉 `SQLReviewCheck`**：它依赖 `component/sheet` 与 `plugin/schema`（会牵入全部方言解析器与 tidb），
   engine 改为规则级调用 `advisor.Check`。`plugin/advisor/sql_review.go` 仅保留规则常量与 payload 解析。
3. **去掉补全 / 诊断 / 语句区间**：删除 `plugin/parser/base/{complete.go,diagnose.go}`、
   `plugin/parser/mysql/{completion.go,diagnose.go}`，并在 `plugin/parser/base/interface.go` 中移除
   对应注册与 `lsp-protocol` 依赖。
4. **去掉 tidb 依赖**：删除 `plugin/parser/tokenizer/tokenizer.go` 中
   `SetLineForMySQLCreateTableStmt` / `matchMySQLTableConstraint`。
5. **精简 `common`**：复制阶段排除 `cel*.go`、`context.go`、`retry.go`、`qb/`（分别依赖 cel-go、
   `generated-go/v1`、backoff、未被引用）；裁剪阶段删除审核链路用不到的应用级工具
   `audit.go`、`config.go`、`config_dev.go`、`config_release.go`、`directory_sync.go`、
   `environment.go`、`password.go`、`risk.go`；并去掉 `ValidatePhone`（phonenumbers）与 `connectrpc` 引用。
6. 同步时排除所有 `*_test.go`、`test-data/`、上游测试辅助文件。

## 同步 / 升级

```bash
cd engine
./scripts/sync-bytebase.sh              # 同步脚本内置的默认版本
./scripts/sync-bytebase.sh 3.23.0       # 指定 tag / 分支 / commit
```

流程：稀疏拉取上游 → 复制子集 → 改写 import 前缀为 `engine/internal/bytebase/` → 重放裁剪 → `gofmt` → `go build` / `go test`。
同步后务必审阅 `git diff` 再提交。

查看是否有新版本：

```bash
cd engine
./scripts/check-upstream.sh          # 对比当前内化版本与上游最新 release
```

`.github/workflows/check-bytebase-upstream.yml` 每天只做检测（发现新版本即失败提醒），
**不会自动修改代码**；确认要升级后，人工执行上面的 `sync-bytebase.sh` 并审阅 diff。

本目录**只放同步来的代码，不要手改**——手改会在下次同步时丢失。
自研规则请写在 [`internal/customrules/`](../customrules/README.md)（同步脚本不触碰）。

## 升级防炸（护栏）

**目标：让上游再变，需要改的自研代码也始终只有那几个文件；且任何破坏都在同步时就被拦住，而不是上线后暴露。**

同步脚本末尾会跑 `go test ./...`，其中包含三道护栏：

- `internal/archguard.TestBytebaseCouplingBoundary`：除适配层（`internal/mysqlparse/`、`internal/server/`）外，
  任何包都不得 import 本目录。新增调用点若绕过适配层，测试直接失败——这是爆炸半径不扩大的根本保证。
- `internal/archguard.TestBytebaseRuleMappingConsistent`：`internal/server/rules.go` 引用的规则类型
  必须同时存在于 `SQLReviewRule_Type` 枚举与 **MySQL 注册表**中，否则失败（否则运行期才报
  `advisor: unknown advisor`）。
- `internal/server.TestReviewRulesAreRegisteredForMySQL`：动态护栏，把 `AuditRole` 全开 / 全关两套配置
  产出的规则逐条交给 `advisor.Check`，确认都能找到实现，并守住基本覆盖面。

配合 `trim-bytebase.py` 的「每处裁剪必须命中」，上游漂移的拦截点如下：

| 漂移类型 | 拦截点 |
| --- | --- |
| 函数 / 类型签名变化（如 `base.Parse`→`ParseStatements`、`Payload` 改 oneof） | 编译期：`go build` |
| 枚举改名 / 删除、规则未注册 | 静态护栏 + 动态护栏：`go test` |
| 上游目录 / 文件结构调整 | 同步期：`trim-bytebase.py` 断言失败 |
| 规则行为 / 默认值变化（能编译但结果变了） | 规则样例测试：`internal/server/check_test.go`（建议按规则扩充） |

因此正确的升级姿势是：**改 `UPSTREAM` 里的 ref → 跑 `sync-bytebase.sh` → 看哪道护栏先响 -> 只改适配层 -> 审 `git diff` 再提交**，
而不是等同步后凭感觉重构。

## 升级注意事项（上游 API 漂移）

升级 Bytebase 版本时，除 `trim-bytebase.py` 的断言外，还需关注这些**跨版本会变**的接口：

- 规则类型：`storepb.SQLReviewRule_Type` **枚举**（3.22.0），不再是以字符串常量暴露的 `SQLReviewRuleType`；
  `Level` 为 `SQLReviewRule_Level`（`SQLReviewRule_ERROR` / `SQLReviewRule_WARNING`）。
- 规则 payload：`SQLReviewRule.Payload` 为 **oneof**（`NumberPayload` / `StringArrayPayload` /
  `CommentConventionPayload` / `NamingPayload` / ...）。
- 审核上下文：`advisor.Context.ParsedStatements []base.ParsedStatement`（旧版为 `AST` + `Statements`）。
- 解析入口：`base.ParseStatements(engine, statement)`（旧版为 `base.Parse`）。

以上映射集中在 [`../server/rules.go`](../server/rules.go)（`AuditRole → 规则`）与
[`../server/check.go`](../server/check.go)（`review`）。

## 为什么内化而不是直接 import

Bytebase 的 advisor / parser 位于 `backend/` 单一应用模块内，直接 import 会把应用级依赖
（各类数据库驱动、connectrpc、cel-go、LSP 等）全部拖入。内化只保留审核所需子集，依赖收敛为
`github.com/bytebase/omni`（解析器）、`github.com/pkg/errors`、`protobuf` / `grpc` 等。
