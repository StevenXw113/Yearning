// Package mongoconn shares MongoDB connection settings between Yearning and its engine.
package mongoconn

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const Timeout = 15 * time.Second

type Config struct {
	Hosts         string
	Port          int
	Username      string
	Password      string
	AuthDatabase  string
	ReplicaSet    string
	TLS           bool
	CA            string
	Certificate   string
	Key           string
	AuthMechanism string
}

// URI accepts comma-separated seeds; seeds without a port use Config.Port.
func (c Config) URI() (string, error) {
	if strings.TrimSpace(c.Hosts) == "" {
		return "", fmt.Errorf("MongoDB 数据源 IP / 节点地址为空")
	}
	if c.Username == "" && c.Password != "" {
		return "", fmt.Errorf("MongoDB 设置密码时必须填写用户名")
	}
	port := c.Port
	if port == 0 {
		port = 27017
	}
	if port < 1 || port > 65535 {
		return "", fmt.Errorf("MongoDB 端口必须在 1 到 65535 之间")
	}
	seeds := strings.Split(c.Hosts, ",")
	for i, seed := range seeds {
		seed = strings.TrimSpace(seed)
		if seed == "" || strings.ContainsAny(seed, "@/?#\\ \t\r\n") {
			return "", fmt.Errorf("MongoDB 节点地址格式错误，请填写逗号分隔的主机名或 IP，可附带端口")
		}
		if host, explicitPort, err := net.SplitHostPort(seed); err == nil {
			parsedPort, err := strconv.Atoi(explicitPort)
			if host == "" || err != nil || parsedPort < 1 || parsedPort > 65535 {
				return "", fmt.Errorf("MongoDB 节点端口必须在 1 到 65535 之间")
			}
			if strings.ContainsAny(host, ":[]") && net.ParseIP(host) == nil {
				return "", fmt.Errorf("MongoDB 节点地址格式错误")
			}
			seeds[i] = net.JoinHostPort(host, strconv.Itoa(parsedPort))
		} else {
			host := seed
			if strings.HasPrefix(seed, "[") && strings.HasSuffix(seed, "]") {
				host = seed[1 : len(seed)-1]
				if net.ParseIP(host) == nil {
					return "", fmt.Errorf("MongoDB IPv6 节点地址格式错误")
				}
			}
			if strings.ContainsAny(host, ":[]") && net.ParseIP(host) == nil {
				return "", fmt.Errorf("MongoDB 节点地址格式错误")
			}
			seeds[i] = net.JoinHostPort(host, strconv.Itoa(port))
		}
	}
	authDB := strings.TrimSpace(c.AuthDatabase)
	if authDB == "" {
		authDB = "admin"
	}
	query := url.Values{"authSource": {authDB}}
	mechanism := strings.TrimSpace(c.AuthMechanism)
	switch mechanism {
	case "":
	case "SCRAM-SHA-1", "SCRAM-SHA-256":
		if c.Username == "" {
			return "", fmt.Errorf("MongoDB 指定认证机制时必须填写用户名")
		}
		query.Set("authMechanism", mechanism)
	default:
		return "", fmt.Errorf("MongoDB 认证机制仅支持自动、SCRAM-SHA-1 或 SCRAM-SHA-256")
	}
	if c.TLS {
		query.Set("tls", "true")
	}
	if replica := strings.TrimSpace(c.ReplicaSet); replica != "" {
		query.Set("replicaSet", replica)
	}
	uri := &url.URL{Scheme: "mongodb", Host: strings.Join(seeds, ","), Path: "/", RawQuery: query.Encode()}
	if c.Username != "" {
		uri.User = url.UserPassword(c.Username, c.Password)
	}
	return uri.String(), nil
}

func (c Config) Options() (*options.ClientOptions, error) {
	uri, err := c.URI()
	if err != nil {
		return nil, err
	}
	opts := options.Client().ApplyURI(uri).
		SetConnectTimeout(10 * time.Second).
		SetServerSelectionTimeout(Timeout).
		SetMaxPoolSize(50).
		SetMinPoolSize(0).
		SetMaxConnIdleTime(time.Minute)
	if c.TLS {
		tlsConfig, err := c.tlsConfig()
		if err != nil {
			return nil, err
		}
		opts.SetTLSConfig(tlsConfig)
	}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("MongoDB 连接配置无效")
	}
	return opts, nil
}

// Connect authenticates and selects a primary before returning the client.
// The caller owns the client and must Disconnect it.
func Connect(c Config) (*mongo.Client, error) {
	opts, err := c.Options()
	if err != nil {
		return nil, err
	}
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	if err := client.Ping(ctx, nil); err != nil {
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_ = client.Disconnect(cleanup)
		return nil, err
	}
	return client, nil
}
