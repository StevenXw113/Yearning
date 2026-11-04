package redischeck

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"engine/api"
)

// 变更类型，取值与工单 Type 对齐（主程序侧 vars.DDL=0 / vars.DML=1）
const (
	KindQuery = -1 // 只读命令，不走工单
	KindDDL   = 0  // 管理/结构类
	KindDML   = 1  // 改数据
)

// 命令表。
//
// engine 是独立 Go module，不能复用主程序 src/lib/rediscmd 里的表，所以维护一份。
// 两处语义必须一致：主程序那份是**执行底线**（不受配置影响），
// 这份是**可按规则集开关**的审核，改任何一边都要同步另一边。
var (
	// 只读命令白名单已收敛到 engine/api（主程序查询页共用一份），见 api.RedisReadCommands。

	// writeCommands 改数据的命令（分类用；安全校验不依赖它）
	writeCommands = map[string]struct{}{
		"SET": {}, "SETEX": {}, "PSETEX": {}, "SETNX": {}, "MSET": {}, "MSETNX": {}, "APPEND": {}, "SETRANGE": {},
		"INCR": {}, "DECR": {}, "INCRBY": {}, "DECRBY": {}, "INCRBYFLOAT": {}, "GETSET": {}, "GETDEL": {}, "GETEX": {},
		"DEL": {}, "UNLINK": {}, "EXPIRE": {}, "PEXPIRE": {}, "EXPIREAT": {}, "PEXPIREAT": {}, "PERSIST": {},
		"RENAME": {}, "RENAMENX": {}, "MOVE": {}, "COPY": {}, "RESTORE": {},
		"HSET": {}, "HSETNX": {}, "HMSET": {}, "HDEL": {}, "HINCRBY": {}, "HINCRBYFLOAT": {},
		"LPUSH": {}, "RPUSH": {}, "LPUSHX": {}, "RPUSHX": {}, "LPOP": {}, "RPOP": {}, "LSET": {}, "LINSERT": {},
		"LREM": {}, "LTRIM": {}, "RPOPLPUSH": {}, "LMOVE": {}, "LMPOP": {},
		"SADD": {}, "SREM": {}, "SPOP": {}, "SMOVE": {}, "SINTERSTORE": {}, "SUNIONSTORE": {}, "SDIFFSTORE": {},
		"ZADD": {}, "ZREM": {}, "ZINCRBY": {}, "ZREMRANGEBYRANK": {}, "ZREMRANGEBYSCORE": {}, "ZREMRANGEBYLEX": {},
		"ZPOPMIN": {}, "ZPOPMAX": {}, "ZMPOP": {}, "ZUNIONSTORE": {}, "ZINTERSTORE": {}, "ZDIFFSTORE": {}, "ZRANGESTORE": {},
		"SETBIT": {}, "BITOP": {}, "BITFIELD": {}, "PFADD": {}, "PFMERGE": {},
		"XADD": {}, "XDEL": {}, "XTRIM": {}, "XACK": {}, "XGROUP": {}, "XCLAIM": {}, "XAUTOCLAIM": {}, "XSETID": {},
		"GEOADD": {}, "GEORADIUS": {}, "GEORADIUSBYMEMBER": {}, "GEOSEARCHSTORE": {},
	}

	// dangerousCommands 影响面是整个实例的命令（RedisForbidDangerous）
	dangerousCommands = map[string]string{
		"FLUSHALL": "禁止 FLUSHALL（清空所有库）",
		"FLUSHDB":  "禁止 FLUSHDB（清空当前库）",
		"SHUTDOWN": "禁止 SHUTDOWN（关闭实例）",
		"SWAPDB":   "禁止 SWAPDB（交换两个库的数据）",
	}

	// adminCommands 只保留 CONFIG / CLIENT：具体子命令的读写区分在规则里做，
	// 只读子命令（CONFIG GET、CLIENT LIST）不该被一刀切拦下。
	adminCommands = map[string]string{
		"CONFIG": "禁止修改实例配置（CONFIG SET / REWRITE / RESETSTAT）",
		"CLIENT": "禁止修改连接状态（CLIENT KILL / PAUSE / UNPAUSE）",
	}

	// scriptCommands 服务端脚本执行（RedisForbidScript）：
	// SCRIPT / FUNCTION 的管理子命令归 RedisForbidScriptManagement。
	scriptCommands = map[string]string{
		"EVAL":       "禁止 EVAL（服务端执行 Lua 脚本）",
		"EVALSHA":    "禁止 EVALSHA（服务端执行 Lua 脚本）",
		"EVAL_RO":    "禁止 EVAL_RO（服务端执行只读 Lua 脚本）",
		"EVALSHA_RO": "禁止 EVALSHA_RO（服务端执行只读 Lua 脚本）",
		"FCALL":      "禁止 FCALL（调用服务端函数）",
		"FCALL_RO":   "禁止 FCALL_RO（调用只读服务端函数）",
	}

	// replicaCommands 主从关系（RedisForbidReplica）
	replicaCommands = map[string]string{
		"REPLICAOF": "禁止 REPLICAOF（改变主从关系）",
		"SLAVEOF":   "禁止 SLAVEOF（改变主从关系，旧名）",
	}

	// blockingCommands 会长时间占用连接的阻塞命令（RedisForbidBlockingCommand）
	blockingCommands = map[string]string{
		"BLPOP":      "禁止 BLPOP（阻塞式弹出，会长时间占用连接）",
		"BRPOP":      "禁止 BRPOP（阻塞式弹出，会长时间占用连接）",
		"BLMOVE":     "禁止 BLMOVE（阻塞式移动）",
		"BRPOPLPUSH": "禁止 BRPOPLPUSH（阻塞式移动）",
		"BLMPOP":     "禁止 BLMPOP（阻塞式弹出）",
		"BZPOPMIN":   "禁止 BZPOPMIN（阻塞式弹出）",
		"BZPOPMAX":   "禁止 BZPOPMAX（阻塞式弹出）",
		"WAIT":       "禁止 WAIT（等待副本确认，会阻塞）",
		"WAITAOF":    "禁止 WAITAOF（等待持久化确认，会阻塞）",
	}

	// subscriptionCommands 持续订阅类命令（RedisForbidSubscription）
	subscriptionCommands = map[string]string{
		"SUBSCRIBE":    "禁止 SUBSCRIBE（订阅会长期占用连接）",
		"PSUBSCRIBE":   "禁止 PSUBSCRIBE（订阅会长期占用连接）",
		"SSUBSCRIBE":   "禁止 SSUBSCRIBE（订阅会长期占用连接）",
		"UNSUBSCRIBE":  "禁止 UNSUBSCRIBE（订阅状态由入口统一管理）",
		"PUNSUBSCRIBE": "禁止 PUNSUBSCRIBE（订阅状态由入口统一管理）",
		"SUNSUBSCRIBE": "禁止 SUNSUBSCRIBE（订阅状态由入口统一管理）",
	}

	// debugCommands / monitorCommands 单独成规则，便于分别配置级别
	debugCommands   = map[string]string{"DEBUG": "禁止 DEBUG（调试命令可造成阻塞或崩溃）"}
	monitorCommands = map[string]string{"MONITOR": "禁止 MONITOR（实时打印所有命令，泄露数据且影响性能）"}

	// aclCommands / moduleCommands / clusterCommands 只在变更子命令上拦，
	// 只读子命令（ACL LIST、CLUSTER INFO 等）放行。
	aclCommands     = map[string]string{"ACL": "禁止修改用户与权限（ACL SETUSER / DELUSER / LOAD 等）"}
	moduleCommands  = map[string]string{"MODULE": "禁止变更模块（MODULE LOAD / LOADEX / UNLOAD）"}
	clusterCommands = map[string]string{"CLUSTER": "禁止修改集群拓扑与 Slot 状态（CLUSTER 变更子命令）"}

	// persistenceCommands 落盘管理（RedisForbidPersistence）
	persistenceCommands = map[string]string{
		"SAVE":         "禁止 SAVE（阻塞式落盘）",
		"BGSAVE":       "禁止 BGSAVE（触发落盘）",
		"BGREWRITEAOF": "禁止 BGREWRITEAOF（触发全量重写）",
	}

	// migrationCommands 迁移/恢复类命令（RedisForbidMigration）
	migrationCommands = map[string]string{
		"MIGRATE": "禁止 MIGRATE（跨实例搬键，转专项审核）",
		"RESTORE": "禁止 RESTORE（用序列化数据覆盖目标键，转专项审核）",
	}

	// connectionCommands 认证与连接状态控制（RedisForbidConnectionControl）
	connectionCommands = map[string]string{
		"AUTH":  "禁止提交 AUTH（认证由审核入口统一完成）",
		"HELLO": "禁止提交 HELLO（协议与认证状态由审核入口统一管理）",
		"QUIT":  "禁止提交 QUIT（连接生命周期由审核入口管理）",
		"RESET": "禁止提交 RESET（连接状态由审核入口统一管理）",
	}

	// transactionCommands 事务控制（RedisForbidWatch / 事务完整性相关规则使用）
	transactionCommands = map[string]string{
		"MULTI":   "MULTI 事务必须作为完整审核单元提交",
		"EXEC":    "EXEC 必须与 MULTI 成对出现",
		"DISCARD": "DISCARD 必须与 MULTI 成对出现",
		"WATCH":   "禁止 WATCH（普通命令入口不保证完整会话语义）",
		"UNWATCH": "禁止 UNWATCH（普通命令入口不保证完整会话语义）",
	}

	// dbCommands 逻辑库切换：DB 权限由 RedisDbScope 判定
	dbCommands = map[string]string{
		"SELECT": "SELECT 切换逻辑库需符合授权的 DB 范围",
	}

	// scriptShellCommands SCRIPT / FUNCTION 是子命令容器：本身不改状态，
	// 具体子命令由 RedisForbidScriptManagement 判定；这里只登记语义。
	scriptShellCommands = map[string]string{
		"SCRIPT":   "SCRIPT 的管理子命令由脚本规则单独审核",
		"FUNCTION": "FUNCTION 的管理子命令由脚本规则单独审核",
	}
)

// inTable 命令是否在表里
func inTable(table map[string]string, name string) bool {
	_, ok := table[name]
	return ok
}

// managementTables 全部管理/安全类命令表（不含只读与写入表）
var managementTables = []map[string]string{
	dangerousCommands, adminCommands, scriptCommands, replicaCommands, blockingCommands,
	subscriptionCommands, debugCommands, monitorCommands, aclCommands, moduleCommands,
	clusterCommands, persistenceCommands, migrationCommands, connectionCommands,
	transactionCommands, dbCommands, scriptShellCommands,
}

// knownCommand 命令是否已登记语义（用于 RedisUnknownCommand）。
// 未登记的命令一律按「语义未知」处理：模块命令（JSON.SET 之类）与厂商扩展都落在这里。
func knownCommand(name string) bool {
	if api.RedisIsReadCommand(name) {
		return true
	}
	if _, ok := writeCommands[name]; ok {
		return true
	}
	for _, t := range managementTables {
		if inTable(t, name) {
			return true
		}
	}
	return false
}

// scriptManagementCommands SCRIPT / FUNCTION 的管理子命令（RedisForbidScriptManagement）
var scriptManagementCommands = map[string]struct{}{
	"LOAD": {}, "FLUSH": {}, "RESTORE": {}, "CREATE": {}, "DELETE": {}, "KILL": {},
}

// Command 一条解析后的 Redis 命令
type Command struct {
	// Name 命令名（首参数，大写归一）
	Name string
	// Args 全部参数（含命令名）
	Args []string
	// Kind 变更类型（KindQuery / KindDDL / KindDML）
	Kind int
	// Keys 受影响的键（源键 + 目标键；只读命令同样会填）
	Keys []string
	// KeysKnown 键是否被完整识别：false 表示命令未登记语义
	KeysKnown bool
}

// Parse 解析一行命令
func Parse(line string) (*Command, error) {
	args, err := Split(line)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("Redis 命令不能为空")
	}
	name := strings.ToUpper(args[0])
	src, dst, known := commandKeys(args)
	return &Command{
		Name:      name,
		Args:      args,
		Kind:      classify(name),
		Keys:      allKeys(src, dst),
		KeysKnown: known,
	}, nil
}

// Split 按 redis-cli 的参数规则切词：空白分隔，双引号内解析 \n \xHH 等转义，
// 单引号内只识别 \' 与 \\。与主程序 src/lib/rediscmd.Split 的实现保持一致。
func Split(line string) ([]string, error) {
	var (
		args  []string
		cur   strings.Builder
		quote byte
		has   bool
	)
	flush := func() {
		if has {
			args = append(args, cur.String())
			cur.Reset()
			has = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote == 0 && (c == ' ' || c == '\t' || c == '\r' || c == '\n'):
			flush()
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
			has = true
		case quote != 0 && c == quote:
			quote = 0
		case quote == '"' && c == '\\' && i+1 < len(line):
			i++
			switch line[i] {
			case 'n':
				cur.WriteByte('\n')
			case 'r':
				cur.WriteByte('\r')
			case 't':
				cur.WriteByte('\t')
			case 'b':
				cur.WriteByte('\b')
			case 'a':
				cur.WriteByte('\a')
			case 'x':
				if i+2 < len(line) {
					var v int
					if _, err := fmt.Sscanf(line[i+1:i+3], "%02x", &v); err == nil {
						cur.WriteByte(byte(v))
						i += 2
						break
					}
				}
				cur.WriteByte('x')
			default:
				cur.WriteByte(line[i])
			}
		case quote == '\'' && c == '\\' && i+1 < len(line) && (line[i+1] == '\'' || line[i+1] == '\\'):
			i++
			cur.WriteByte(line[i])
		default:
			cur.WriteByte(c)
			has = true
		}
	}
	if quote != 0 {
		return nil, errors.New("命令里的引号没有闭合")
	}
	flush()
	return args, nil
}

// classify 分类：危险/管理/脚本/主从/事务等都按结构变更处理，
// 否则它们会落进「不是变更命令」那条分支——既拦不住也说不清理由。
func classify(name string) int {
	if _, ok := writeCommands[name]; ok {
		return KindDML
	}
	if api.RedisIsReadCommand(name) {
		return KindQuery
	}
	// 未知命令按变更处理：宁可走审批，也别把写命令当查询放行
	return KindDDL
}

// ---------- 规则用的取值helper ----------

// Arg 取第 i 个参数（含命令名，i 从 0 起）；越界返回空串
func (c *Command) Arg(i int) string {
	if i < 0 || i >= len(c.Args) {
		return ""
	}
	return c.Args[i]
}

// ArgsFrom 取第 i 个参数起的全部参数
func (c *Command) ArgsFrom(i int) []string {
	if i < 0 || i >= len(c.Args) {
		return nil
	}
	return c.Args[i:]
}

// ArgCount 参数个数（含命令名）
func (c *Command) ArgCount() int { return len(c.Args) }

// IntArg 把第 i 个参数当整数取；不是整数返回 0、false
func (c *Command) IntArg(i int) (int64, bool) {
	s := c.Arg(i)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// HasAnyArg 参数里是否出现某个选项（大小写不敏感），
// 用于判断 SET 是否带了 EX / PX / EXAT / KEEPTTL 这类过期选项。
func (c *Command) HasAnyArg(names ...string) bool {
	for _, a := range c.Args {
		for _, n := range names {
			if strings.EqualFold(a, n) {
				return true
			}
		}
	}
	return false
}


