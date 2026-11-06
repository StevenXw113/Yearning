package rediscmd

import "engine/api"

// 受影响键与引号拼接的实现都在 engine/api（与主程序共用一份）。

// AffectedKeys 返回一条命令可能改动的键，用于抓前镜像。
func AffectedKeys(args []string) []string { return api.RedisAffectedKeys(args) }

// Quote 按 redis-cli 规则给参数加引号（与 Split 互逆）。
func Quote(s string) string { return api.QuoteRedisArg(s) }

// QuoteAll 批量加引号并用空格连接。
func QuoteAll(items []string) string { return api.QuoteRedisArgs(items) }

// CommandText 把参数拼回一行命令。
func CommandText(args []string) string { return api.RedisCommandText(args) }
