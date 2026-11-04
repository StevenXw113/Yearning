package model

import "testing"

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
