# internal/bytebase

本目录内化（vendor-in）了 **Bytebase 的 SQL 解析器（MySQL / MariaDB / PostgreSQL）与 MySQL、PG 审核规则**，供 engine 的审核链路直接调用。
engine 自身只做「拆分 → 逐条审核 → 结果映射」，不重复实现任何规则，也不依赖第三方封装 module。

- 上游仓库：`https://github.com/bytebase/bytebase`
- 当前同步版本：见 [`UPSTREAM`](./UPSTREAM)（默认取最新 release tag）
- 许可：MIT Expat，见 [`LICENSE`](./LICENSE)（上游 `enterprise` 相关目录未纳入）
- 目录布局**镜像上游 `backend/`**，便于对照升级：

```
engine/internal/bytebase/          # 对应 bytebase/backend/
├── plugin/advisor/                # 审核规则框架（advisor.go、code）
│   ├── sql_review.go              # 仅保留规则类型常量与模板解析（已砍 SQLReviewCheck）
│   ├── mysql/                     # MySQL 全部审核规则实现
│   └── pg/                        # PostgreSQL 审核规则实现
├── plugin/parser/base/            # 解析器接口、AST、拆分、错误类型
├── plugin/parser/mariadb/         # MariaDB 解析（3.24.0 起 MySQL 解析器依赖它）
├── plugin/parser/mysql/           # MySQL 解析 / 拆分 / explain 估算
├── plugin/parser/pg/              # PostgreSQL 解析 / 拆分
├── common/                        # 审核链路用到的通用工具（已裁剪）
├── store/model/                   # 元数据模型
├── generated-go/store/            # protobuf 生成代码（只留 5 个审核链路用到的文件）
├── UPSTREAM                       # 同步来源与版本（脚本自动生成）
├── README.md
└── LICENSE
```

## 与上游的差异（裁剪清单）

每一处裁剪都写在 [`../../scripts/trim-bytebase.py`](../../scripts/trim-bytebase.py) 中，且**必须命中**，
否则同步脚本报错退出——说明上游结构变了，需要人工调整脚本，而不是静默产出一份编译不过的内化代码。

主要裁剪：

1. **只保留 MySQL 与 PG**：删除 `plugin/advisor/{bigquery,mssql,oceanbase,oracle,redshift,snowflake,tidb}`
   （`bigquery` 是 3.24.0 新增，依赖未内化的 `parser/bigquery` + `parser/googlesql`）。
   MySQL 解析器自 3.24.0 起依赖 `parser/mariadb`，故该目录一并保留，但不保留它的 `diagnose.go`。
2. **砍掉 `SQLReviewCheck`**：它依赖 `component/sheet` 与 `plugin/schema`（会牵入全部方言解析器与 tidb），
   engine 改为规则级调用 `advisor.Check`。`plugin/advisor/sql_review.go` 仅保留规则常量与 payload 解析。
3. **只留三台解析机器**：engine 审核链路只用 `ParseStatements` / `SplitMultiSQL` / `GetStatementTypes`
   （外加各方言包的 `GetOmniNode` 与 stats.go 的 `EstimateAffectedRowsFromExplainJSON`）。
   补全 / 诊断 / 语句区间（lsp-protocol）、query span、DML→SELECT、备份还原、资源变更、
   编辑器校验（`ValidateSQLForEditor`）与 `IsAllDML`（golang-lru + xxh3 缓存）整机删除；
   `base/interface.go` 收敛为三台注册机器，错误构造改用 stdlib `fmt.Errorf`。
   死机器里个别仍被活代码引用的纯函数（`dmlKeywordStart`、pg 的 6 个 AST 辅助 +
   `explainedText`、`ListDatabaseNamesFunc` 类型）随迁到存活文件，`pg/query_type.go`
   冻结为只含活函数的副本。`plugin/parser/tokenizer/` 零引用，整包不拷。
4. **砍死规则**：`EnablePriorBackup` / `EnableGhost` 全仓库从未置位，prior-backup 两条规则的
   第一道门恒为假（`Check` 永远直接返回 nil），连同零调用的 `builtin_rules.go`（`GetBuiltinRules`）
   一并删除。RULES.md 清单同步把它们记为不存在。
5. **protobuf 只留 5 个文件**：`generated-go/store` 保留 `advice` / `common` / `database` /
   `plan_check_run`（ChangedResource 家族）/ `review_config`（SQLReviewRule 家族）；
   其余 30+ 个应用级 proto（issue / plan / task / project / user / setting / …）与全部
   `*_equal.pb.go`（Equal 方法零调用）删除——genproto、golang-lru、xxh3 三个直接依赖随之消失。
6. **精简 `common`**：复制阶段排除 `cel*.go`、`context.go`、`retry.go`、`qb/`（分别依赖 cel-go、
   `generated-go/v1`、backoff、未被引用）；裁剪阶段删除审核链路用不到的应用级工具
   `audit.go`、`config.go`、`config_dev.go`、`config_release.go`、`directory_sync.go`、
   `environment.go`、`password.go`、`risk.go`、`resource_name.go`（38 个资源名解析器，调用方已删）、
   `proto.go`（`SanitizeUTF8Message` 零调用）；并去掉 `ValidatePhone`（phonenumbers）与 `connectrpc` 引用。
7. 同步时排除所有 `*_test.go`、`test-data/`、advisor 的 `test/` fixture（约 16MB YAML）、
   上游测试辅助文件。

## 同步 / 升级

```bash
cd engine
./scripts/sync-bytebase.sh              # 同步脚本内置的默认版本
./scripts/sync-bytebase.sh 3.24.0       # 指定 tag / 分支 / commit
```

流程：稀疏拉取上游 → 复制子集 → 改写 import 前缀为 `engine/internal/bytebase/` → 重放裁剪 → `gofmt` → `go build` / `go test`。
同步后务必审阅 `git diff` 再提交。

查看是否有新版本：

```bash
cd engine
./scripts/check-upstream.sh          # 对比当前内化版本与上游最新 release
```

页面「设置 → 审核规则 → 检测上游更新」也会告诉你有没有新版本（拿到新版本号后在 `engine/` 下执行
`./scripts/upgrade-engine.sh` 即可自动同步，先加 `--preview` 可以只预演、不动工作区）。
**检测与预演都不会自动改代码**，确认后再正式执行并审阅 diff。

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
`github.com/bytebase/omni`（解析器）、`github.com/pkg/errors`、`protobuf` 等。
