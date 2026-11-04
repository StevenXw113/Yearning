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

// 连接参数取文档推荐值，改动会影响所有 Redis 数据源，用断言钉住。
func TestRedisOptionsFollowsCloudDefaults(t *testing.T) {
	s := &CoreDataSource{IP: "127.0.0.1", Port: 6379, Username: "app"}
	opt := RedisOptions(s, "pwd")
	if opt.DialTimeout != 2*time.Second || opt.ReadTimeout != 2*time.Second || opt.WriteTimeout != 2*time.Second {
		t.Errorf("超时不符合推荐值: dial=%v read=%v write=%v", opt.DialTimeout, opt.ReadTimeout, opt.WriteTimeout)
	}
	if opt.PoolTimeout != 3*time.Second || opt.PoolSize != 200 || opt.MaxRetries != 3 {
		t.Errorf("连接池参数不符合推荐值: %+v", opt)
	}
	if opt.Username != "app" || opt.Password != "pwd" {
		t.Errorf("ACL 用户名/口令未透传: %+v", opt)
	}

	cluster := RedisClusterOptions(s, "pwd")
	if cluster.MaxRedirects != 3 || cluster.PoolSize != 200 || cluster.ConnMaxIdleTime != 30*time.Minute {
		t.Errorf("集群参数不符合推荐值: %+v", cluster)
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
