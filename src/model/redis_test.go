package model

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// 代理实例（腾讯云 nj-crs-*.sql.tencentcdb.com）同样报 cluster_enabled:1，
// 但槽位给的是内网 IP：不能据此切集群客户端，否则所有命令都连不上。
func TestSlotsMatchHost(t *testing.T) {
	slots := []redis.ClusterSlot{{
		Start: 0, End: 5460,
		Nodes: []redis.ClusterNode{{ID: "x", Addr: "10.80.10.16:6379"}},
	}}
	if slotsMatchHost(slots, "nj-crs-5nhsp5wa.sql.tencentcdb.com") {
		t.Error("内网地址不应判定为直连")
	}
	if !slotsMatchHost(slots, "10.80.10.16") {
		t.Error("槽位地址与配置一致时应判定为直连")
	}
}

// INFO keyspace 只返回有键的库：托管实例 databases 常配成 256，不能把空库也列进树里
func TestKeyspaceDatabases(t *testing.T) {
	info := "# Keyspace\r\ndb1:keys=2,expires=0,avg_ttl=0\r\ndb0:keys=3,expires=1,avg_ttl=0\r\n"
	got := keyspaceDatabases(info)
	if len(got) != 2 || got[0] != "db0" || got[1] != "db1" {
		t.Fatalf("应按库号排序返回 db0/db1, got %v", got)
	}
	if dbs := keyspaceDatabases("# Keyspace\r\n"); len(dbs) != 0 {
		t.Errorf("空 keyspace 不应返回库, got %v", dbs)
	}
}

// 连接参数取文档推荐值，改动会影响所有 Redis 数据源，用断言钉住。
func TestRedisOptionsFollowsCloudDefaults(t *testing.T) {
	s := &CoreDataSource{IP: "127.0.0.1", Port: 6379, Username: "app"}
	opt := RedisOptions(s, "pwd")
	// 读放宽到 5s（大 key 的 HGETALL、稀疏 SCAN 在 2s 下会假性超时），写保持 2s
	if opt.DialTimeout != 2*time.Second || opt.ReadTimeout != 5*time.Second || opt.WriteTimeout != 2*time.Second {
		t.Errorf("超时不符合推荐值: dial=%v read=%v write=%v", opt.DialTimeout, opt.ReadTimeout, opt.WriteTimeout)
	}
	if opt.PoolTimeout != 3*time.Second || opt.PoolSize != 200 || opt.MaxRetries != 3 {
		t.Errorf("连接池参数不符合推荐值: %+v", opt)
	}
	// 客户端进程级复用后空闲连接会一直留着，上限不能再放到 200
	if opt.MaxIdleConns != 20 {
		t.Errorf("MaxIdleConns 应收敛到 20, got %d", opt.MaxIdleConns)
	}
	if opt.Username != "app" || opt.Password != "pwd" {
		t.Errorf("ACL 用户名/口令未透传: %+v", opt)
	}

	cluster := RedisClusterOptions(s, "pwd")
	if cluster.MaxRedirects != 3 || cluster.PoolSize != 200 || cluster.ConnMaxIdleTime != 30*time.Minute {
		t.Errorf("集群参数不符合推荐值: %+v", cluster)
	}
}

// 进程级客户端池：同一数据源×库只建一次；配置变更换新；删除数据源后释放。
// dial 用不实际建连的 redis.NewClient（命令才会拨号），测试不需要真实实例。
func TestRedisClientPoolReuseAndInvalidate(t *testing.T) {
	dials := 0
	p := &redisClientPool{
		entries: map[redisClientKey]redisPoolEntry{},
		dial: func(*CoreDataSource, string, int) (RedisConn, error) {
			dials++
			return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}), nil
		},
	}
	s := &CoreDataSource{SourceId: "s1", IP: "10.0.0.1", Port: 6379}

	first, err := p.get(s, 0)
	if err != nil || first == nil {
		t.Fatalf("首次取客户端失败: %v", err)
	}
	again, err := p.get(s, 0)
	if err != nil {
		t.Fatal(err)
	}
	if dials != 1 {
		t.Errorf("同一数据源×库不应重复建连，dials=%d", dials)
	}
	if first != again {
		t.Error("同一数据源×库应返回同一客户端")
	}

	if _, err := p.get(s, 3); err != nil {
		t.Fatal(err)
	}
	if dials != 2 {
		t.Errorf("换库必须换客户端，dials=%d", dials)
	}

	// 地址变更：同一条目换新客户端，旧的不再被复用
	moved := &CoreDataSource{SourceId: "s1", IP: "10.0.0.2", Port: 6379}
	movedClient, err := p.get(moved, 0)
	if err != nil || movedClient == first {
		t.Fatalf("配置变更后应换新客户端: client=%v err=%v", movedClient, err)
	}
	if len(p.entries) != 2 {
		t.Errorf("条目数应为 数据源×库 = 2, got %d", len(p.entries))
	}

	p.drop("s1")
	if len(p.entries) != 0 {
		t.Errorf("删除数据源后条目应清空, got %d", len(p.entries))
	}
}

// 口令变化必须体现在指纹里，否则改密后还会复用旧连接。
func TestRedisClientSignatureTracksCredentials(t *testing.T) {
	base := &CoreDataSource{IP: "10.0.0.1", Port: 6379, Username: "app"}
	if redisClientSignature(base, "p1", 0) == redisClientSignature(base, "p2", 0) {
		t.Error("口令不同应产生不同指纹")
	}
	if redisClientSignature(base, "p1", 0) == redisClientSignature(base, "p1", 1) {
		t.Error("库不同应产生不同指纹")
	}
}

func TestRedisClusterEnabled(t *testing.T) {
	cluster := "# Cluster\r\ncluster_enabled:1\r\ncluster_state:ok\r\n"
	if !RedisClusterEnabled(cluster) {
		t.Fatal("cluster_enabled:1 应判定为集群")
	}
	single := "# Cluster\r\ncluster_enabled:0\r\n"
	if RedisClusterEnabled(single) {
		t.Fatal("cluster_enabled:0 不应判定为集群")
	}
	// 非集群实例对 CLUSTER INFO 直接报错，调用方拿不到字符串，这里只守解析
	if RedisClusterEnabled("") {
		t.Fatal("空串不应判定为集群")
	}
}
