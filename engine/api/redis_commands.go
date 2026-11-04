package api

import "strings"

// RedisReadCommands 只读命令白名单。
//
// 主程序查询页（执行底线，不受规则集影响）与引擎审核（可按开关配置）共用这一份：
// 两边各维护一张表，加命令时漏改一边就会出现「查询页放行、检测页判成变更命令」这种
// 自相矛盾的结果。刻意用白名单而不是黑名单——Redis 命令两百多个还在增加，
// 漏一个就等于查询页能执行写命令。不在表里的一律按变更处理。
var RedisReadCommands = map[string]struct{}{
	// string
	"GET": {}, "MGET": {}, "STRLEN": {}, "GETRANGE": {}, "SUBSTR": {},
	// 通用
	"EXISTS": {}, "TYPE": {}, "TTL": {}, "PTTL": {}, "SCAN": {}, "RANDOMKEY": {},
	"DUMP": {}, "TOUCH": {}, "OBJECT": {}, "MEMORY": {},
	// hash
	"HGET": {}, "HMGET": {}, "HGETALL": {}, "HKEYS": {}, "HVALS": {}, "HLEN": {}, "HEXISTS": {}, "HSCAN": {}, "HRANDFIELD": {},
	// list
	"LRANGE": {}, "LLEN": {}, "LINDEX": {}, "LPOS": {},
	// set
	"SMEMBERS": {}, "SCARD": {}, "SISMEMBER": {}, "SMISMEMBER": {}, "SRANDMEMBER": {}, "SSCAN": {}, "SDIFF": {}, "SINTER": {}, "SUNION": {}, "SINTERCARD": {},
	// zset
	"ZRANGE": {}, "ZRANGEBYSCORE": {}, "ZREVRANGE": {}, "ZREVRANGEBYSCORE": {}, "ZCARD": {}, "ZSCORE": {}, "ZMSCORE": {},
	"ZRANK": {}, "ZREVRANK": {}, "ZCOUNT": {}, "ZSCAN": {}, "ZRANGEBYLEX": {}, "ZREVRANGEBYLEX": {}, "ZRANDMEMBER": {},
	// 位图 / HyperLogLog / 流（读取）
	"GETBIT": {}, "BITCOUNT": {}, "BITPOS": {}, "PFCOUNT": {},
	"XLEN": {}, "XRANGE": {}, "XREVRANGE": {}, "XREAD": {}, "XINFO": {},
	// 地理
	"GEOPOS": {}, "GEODIST": {}, "GEOHASH": {}, "GEOSEARCH": {},
	// 服务端信息（只读）
	"INFO": {}, "DBSIZE": {}, "PING": {}, "ECHO": {}, "TIME": {}, "LOLWUT": {},
	"COMMAND": {}, "SLOWLOG": {}, "LATENCY": {},
}

// RedisIsReadCommand 命令是否只读（命令名大小写不敏感）
func RedisIsReadCommand(name string) bool {
	_, ok := RedisReadCommands[strings.ToUpper(strings.TrimSpace(name))]
	return ok
}
