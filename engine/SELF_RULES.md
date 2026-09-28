# 自研审核规则清单（自动生成，勿手改）

本文件由 `internal/archguard/self_rules_manifest_test.go` 生成并核对：

- 更新：`UPDATE_SELF_RULES_MANIFEST=1 go test ./internal/archguard -run TestSelfRule`
- 核对：`go test ./internal/archguard -run TestSelfRule`（CI 每次都会跑）

自研规则 = 上游 bytebase 没有、本仓库自己实现的规则，分两处存放：

- `internal/customrules`：SQL 侧（上游虽有 SQL 规则，但这几条静态规则上游没有实现）
- `internal/mongocheck`：MongoDB 侧（上游完全没有 Mongo 审核能力，
  `common.EngineSupportSQLReview(MONGODB)` 为 false，advisor 目录里也没有 mongodb 方言）

每条规则的开关都必须在 `enginev1.AuditRole` 里存在：否则规则页配不出来、
`rule_levels` 里的级别也取不到（引擎按字段名查 map）。这条由
`TestSelfRuleSwitchesExistInAuditRole` 核对，反方向由 `TestMongoSwitchesAreImplemented` 兜住。

上游规则清单见同目录的 `RULES.md`（那份管「上游新增的有没有被漏掉」，与本文件互补）。

## SQL 侧 · internal/customrules（2 条）

- `forbid-drop` — 开关：`DDLEnableDropDatabase`、`DDLEnableDropTable`；禁止 DROP DATABASE / DROP TABLE（受「允许删除数据库/表」两个开关控制）
- `forbid-truncate` — 开关：`DDLForbidTruncate`；禁止 TRUNCATE（不可回滚，如需清空数据请改用 DELETE 并走审批）

## MongoDB 侧 · internal/mongocheck（7 条）

- `MongoForbidAdminCommand` — 开关：`MongoForbidAdminCommand`；禁止 createUser / dropUser / grantRolesToUser / createRole 等账号与权限管理命令
- `MongoForbidDangerous` — 开关：`MongoForbidDangerous`；禁止 dropDatabase / eval / mapReduce 等危险命令
- `MongoForbidDropCollection` — 开关：`MongoForbidDropCollection`；禁止 drop / renameCollection 集合
- `MongoForbidEmptyFilter` — 开关：`MongoForbidEmptyFilter`；禁止无 filter 的 update / delete / findAndModify（否则作用于整个集合）
- `MongoForbidImmutableId` — 开关：`MongoForbidImmutableId`；禁止修改 _id（不可变字段：$set / $unset / $rename / 替换文档）
- `MongoForbidSystemCollection` — 开关：`MongoForbidSystemCollection`；禁止对 system.* 集合（system.users / system.profile / system.js / system.views）做变更
- `MongoForbidWhere` — 开关：`MongoForbidWhere`；禁止 $where（把脚本推到服务端执行）

## 执行期限制 · 非静态审核（1 条）

- `MongoMaxAffectRows` — 开关：`MongoMaxAffectRows`；单次 update / delete 允许命中的文档数上限（0 = 不限），由主程序抓取前镜像时判定
