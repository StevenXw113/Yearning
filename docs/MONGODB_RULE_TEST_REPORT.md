# MongoDB 规则测试报告

测试日期：2026-11-28。环境：Windows，Go 1.27.1，Vitest 5.0.5。

## 结果

| 范围 | 结果 |
| --- | --- |
| 引擎完整测试 `engine: go test ./... -count=1` | 13 个含测试包通过；517 个测试项通过，6 个跳过；数量包含子测试；已在写入修复后重跑 |
| 主程序 MongoDB 相关后端 | 11 个包通过；156 个测试项通过，3 个跳过；数量包含子测试 |
| 前端规则及 MongoDB 数据源配置 | 5 个测试文件、21 项通过 |
| 真实 MongoDB 只读规则集成 | 7 条规则 × 3 种等级全部通过，检查命中和安全放行 |
| 真实 MongoDB 写入、DDL、权限及回滚 | 24 个子场景全部通过；随机隔离集合、用户及角色已清理 |
| 真实 MongoDB 连接认证 | 副本集 2 个成员，认证成功；业务库 `test` 可读，集合数为 0 |
| 差异格式检查 | `git diff --check` 通过，Git 提示现有文件的 LF/CRLF 转换 |

引擎默认运行跳过真实连接及真实规则集成；本次另行指定只读和写入测试名运行并通过。主程序后端、前端及覆盖率结果来自上一轮；本轮修改后重跑了完整 engine 和真实写入集成。

## 静态规则矩阵

`engine/mongo/rules_matrix_test.go` 按公开 manifest 核对 23 条稳定规则 ID；新增规则而没有测试样例会使测试失败。

每条规则覆盖命中与安全命令、开启与关闭、`error/warn/observe`、旧版 API 配置和 v2 规则集、JSON 序列化往返，以及旧版默认等级。对规则声明的每个适用模式分别运行；四条共用性能规则同时覆盖查询与变更。三个硬底线验证关闭或降级仍要求 DBA 特批。

| 分类 | 规则 ID |
| --- | --- |
| 安全 | `mongo.empty_filter`、`mongo.dangerous_command`、`mongo.where`、`mongo.collection.drop`、`mongo.collection.rename`、`mongo.system_collection`、`mongo.account.privilege`、`mongo.immutable_id`、`mongo.upsert`、`mongo.multi_write` |
| 结构 | `mongo.capped_convert`、`mongo.coll_mod`、`mongo.index_key_limit`、`mongo.index_name_spec`、`mongo.collection_prefix`、`mongo.max_collection_name_len` |
| 性能 | `mongo.regex_unanchored`、`mongo.negation_operator`、`mongo.or_clause`、`mongo.large_in_list` |
| 查询 | `mongo.query.no_filter`、`mongo.query.no_lookup`、`mongo.query.no_limit` |

索引键数、集合名长度、`$in` 数量覆盖阈值内及超过阈值的情形。已有测试还覆盖批次子句、`findAndModify`、Extended JSON 整数、查询/变更模式隔离、无效规则集导入和规则元数据一致性。

新增 `engine/mongoguard/query_test.go` 检查只读白名单、explain 解包、写命令拒绝、`$out/$merge`、嵌套脚本操作符、重复 JSON 字段、尾随 JSON、非法 ObjectID，以及命令字段顺序和 int64 精度。

## 发现并修复的问题

完整 DBA 特批的 drop、rename、账号权限命令，审核返回 `Allowed=true`，但原始 Findings 保留硬底线命中作为审计证据。执行前复核再次直接检查 Findings，错误地提示“DBA 特批未完成”。

新增回归测试先复现 5 个失败场景：三个命令已批准，以及已批准的 drop 仍有普通 warn/observe 命中。修复执行前复核后，以审核放行结果识别已解决的硬底线；普通 error 继续阻断，warn/observe 提示保留，审批审计证据保留。

回归还验证缺失、拒绝、缺少审批人、缺少理由及空白审批字段均不能放行。这组审批矩阵只在内存中审核；下方新增的真实测试另外在随机隔离资源上执行已批准的 drop、rename 和权限命令。

## 定向语句覆盖率

| 包 | 覆盖率 |
| --- | ---: |
| `engine/internal/mongocheck` | 88.4% |
| `engine/ruleset` | 78.8% |
| `engine/mongoguard` | 100.0% |
| `engine/mongo` | 37.6% |
| `engine/internal/mongodb` | 35.4% |
| `engine/internal/server` | 49.9% |
| `engine/mongoconn` | 83.0% |

这些是包内语句覆盖率，不能视为所有输入或执行路径已覆盖。引擎全包覆盖率运行曾遇到 `covdata/testmain` 工具链错误；定向包在沙箱外运行覆盖率测试通过。此表为上一轮的定向覆盖率，未用本轮真实写入测试重新统计。真实数据源验证结果来自单独运行的只读和写入集成。

## 真实数据源验证

使用已提供的双节点副本集和 `admin` 认证库。密码只通过测试进程环境提供，不写入代码或报告。

`TestMongoRulesReadOnlyIntegration` 在随机命名、未创建的集合命名空间执行查询。7 条查询相关规则各测试三种等级：error 命中被拦；warn/observe 命中可以执行；安全命令可以执行。静态审核前后检查命令计数，确认审核本身不访问数据库。驱动共记录 38 条命令，逐条检查只允许 ping、集合列表、find、aggregate 和游标关闭；本次集合数仍为 0。

该验证能证明规则决策、真实认证和只读命令兼容性，不能证明非空数据上的扫描成本、返回内容或写入行为。

## 真实写入、DDL、权限和回滚补测

新增 `engine/mongo/live_write_test.go: TestMongoWriteLimitsIntegration`。测试要求 `MONGO_TEST_WRITE=1` 显式开启，通过公开 `Engine.ExecuteDefinition` 执行命令，因此同时验证规则集 `Settings.MaxAffectRows`、执行审核、实际写入和回滚结果。连接密码和临时用户密码均不落盘；临时密码由随机数生成，仅在内存使用。

每轮创建 96 位随机后缀的集合、用户和角色，只清理创建成功且归本轮所有的资源，不预先 drop 固定集合。清理使用独立超时；子测试结束检查集合不存在，权限测试通过 `usersInfo` / `rolesInfo` 确认用户和角色已删除。最终检查本轮前缀下集合为零。未修改已有业务集合或账号。

| 场景 | 真实结果 |
| --- | --- |
| update / delete，规则上限 3 | 命中 2 / 3 条均执行成功，返回对应影响行数并完整回滚；命中 4 条执行前阻断 |
| update / delete，快照上限 1000 | 命中 1000 条执行成功，数据库内容核对通过，生成的回滚恢复全部 1000 条文档 |
| update / delete，命中 1001 条 | 规则上限为 0 或 2000 都在写入前阻断；0 仅关闭规则行数限制，不关闭快照上限 |
| 多个不相交条件累计 | 每个条件单独低于阈值，合并后 4 > 3 或 1001 > 1000，均阻断；更新和删除都覆盖 |
| 重叠更新条件 | 3 个独立文档保留 3 条前镜像；两条更新子句响应累计 `n=6`，回滚完整恢复 |
| 无变化更新 | 命中 3 条但字段值未变化，`Affected=3`；当前口径取匹配数 `n`，不是 `nModified` |
| 不同类型的 `_id` | 整数 1 和字符串 "1" 分别计数；规则上限 1 阻断，上限 2 删除成功并完整恢复两条 |
| insert | 3 条插入遇上限 2 被阻断；上限 3 执行成功，返回 3，生成的回滚删除本次插入文档 |
| findAndModify | 更新、删除、`new:true` 与响应投影场景均恢复完整原文档；未命中返回 0；upsert 返回 1 并可回滚删除新增文档 |
| 写入失败 | 重复 `_id` 的 insert 返回失败，未误报成功 |
| DDL | 真实创建集合、创建索引、删除索引及生成回滚重建；rename 无审批阻断，批准后执行并用逆向 rename 恢复；drop 无审批阻断，批准后删除测试集合 |
| 权限 | 创建仅允许读取本次集合的临时角色和无角色用户；实际登录核对无权限拒绝 → 授权后可读 → 撤权后拒绝；最终删除用户及角色并查询确认不存在 |

所有上限阻断场景同时检查命令监控计数和完整文档内容，确认阻断后未向 MongoDB 发送对应的写命令，数据未改变。24 个子场景全部通过，最终真实运行约 37 秒。

本次真实测试先复现并修复了三个问题：

1. 插入不抓取前镜像，旧实现以零作为待写入数量，导致绕过 `MaxAffectRows`。现按 `documents` 数量做执行前校验，并新增无需连接数据库的阻断回归测试。
2. 前镜像以 `fmt.Sprint(_id)` 去重，把整数 1 与字符串 "1" 混为一条，导致超限漏拦和回滚丢文档。现按 BSON 类型与原始值区分身份。
3. `findAndModify` 没有生成回滚，未命中时还把 `value:null` 误计为 1。现请求完整原文档作为原子响应，生成更新/删除回滚；upsert 按响应中的新增 `_id` 生成删除回滚；优先读取 `lastErrorObject.n` 统计影响行数。执行接口只返回影响行数及回滚元数据，不向调用方返回该命令的文档响应。

生成式自动回滚已验证 insert/update/delete/findAndModify 和索引操作；rename 的恢复由测试提交逆向命令，drop 及权限操作仍不会自动生成回滚。本次没有给这些操作新增自动回滚能力。

## 复现命令

常规回归不设置 `MONGO_TEST_URI` / `MONGO_TEST_DB`。

```powershell
# 仓库根目录
 go test ./src/engine ./src/handler/personal ./src/handler/fetch ./src/handler/order/audit ./src/handler/manage/roles ./src/handler/manage/db ./src/lib/customrule ./src/lib/calls ./src/lib/mongodb ./src/lib/dbengine ./src/model -count=1

# engine 目录
 go test ./... -count=1
 go test ./internal/mongocheck ./ruleset ./mongo ./mongoguard ./internal/mongodb ./internal/server ./mongoconn -count=1 -cover

# front 目录
 pnpm exec vitest run src/views/manager/rules/__tests__ src/apis/__tests__/rules.test.ts src/views/manager/db/__tests__/db.mongo.test.ts
```

真实只读集成需先在进程环境提供 `MONGO_TEST_URI`，可选 `MONGO_TEST_DB`，然后在 engine 目录执行：

```powershell
go test ./mongo ./mongoconn -run '^(TestMongoRulesReadOnlyIntegration|TestConnectIntegration)$' -count=1 -v
```

真实写入补测需在测试进程环境提供 `MONGO_TEST_URI`、`MONGO_TEST_DB`，并显式设置写入开关，在 engine 目录只执行此测试：

```powershell
$env:MONGO_TEST_WRITE = '1'
try {
    go test ./mongo -run '^TestMongoWriteLimitsIntegration$' -count=1 -v
} finally {
    Remove-Item Env:MONGO_TEST_WRITE,Env:MONGO_TEST_URI,Env:MONGO_TEST_DB -ErrorAction SilentlyContinue
}
```

不要把凭据写入报告或仓库。设置真实数据源 URI 后只运行上述指定测试，不运行整套测试；已有 `TestExecuteRollbackIntegration` 使用固定集合并先 drop，该旧测试未执行。本次使用新的随机隔离写入测试。

## 未覆盖范围和已知限制

- 本次覆盖代表性的真实写入、集合/索引 DDL 和临时用户权限生命周期；未穷举 collMod、capped 集合及所有角色权限命令。
- `MongoMaxAffectRows` 不在 23 条静态规则 manifest 中。插入按文档数限制，普通更新/删除按执行前去重的命中文档数限制；重叠子句返回的累计影响次数可能大于这个去重数量。固定 1000 限制的是前镜像条数，不是快照字节数。
- 单条 update（`multi:false`）和 delete（`limit:1`）仍按完整过滤器抓取前镜像，可能保守阻断。普通 update 的 upsert、大批量 insert、批次部分成功/失败和并发写入未验证；前镜像查询与普通更新/删除不是事务，不能将顺序执行的边界测试当作并发条件下的硬上限保证。
- 现有 `_id` 静态规则只解析对象形式的更新文档；聚合管道更新数组中的 `_id` 变更仍依赖 MongoDB 服务端校验，本次没有扩展这部分实现。
- 未进行非空业务数据的性能压测、浏览器人工交互或华为云 DDS 实例联调。
- 前端存在 Vite 配置加载方式的既有提示，不影响本次 21 项测试通过。
