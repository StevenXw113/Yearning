package mongoconn

import (
	"context"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestConnectionOptions(t *testing.T) {
	cases := []struct {
		name   string
		config Config
		hosts  []string
	}{
		{"legacy single host", Config{Hosts: "localhost", Port: 27018}, []string{"localhost:27018"}},
		{"default port", Config{Hosts: "127.0.0.1"}, []string{"127.0.0.1:27017"}},
		{"replica seeds", Config{Hosts: " node-a:27018, node-b, node-c:27019 ", Port: 27020, ReplicaSet: "rs-test"}, []string{"node-a:27018", "node-b:27020", "node-c:27019"}},
		{"IPv6", Config{Hosts: "[::1]:27018, [2001:db8::1],2001:db8::2"}, []string{"[::1]:27018", "[2001:db8::1]:27017", "[2001:db8::2]:27017"}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			opts, err := tt.config.Options()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(opts.Hosts, tt.hosts) {
				t.Fatalf("hosts = %v, want %v", opts.Hosts, tt.hosts)
			}
			if tt.config.ReplicaSet != "" && (opts.ReplicaSet == nil || *opts.ReplicaSet != tt.config.ReplicaSet) {
				t.Fatal("replica set lost")
			}
			if opts.Auth != nil {
				t.Fatal("anonymous configuration unexpectedly carries credentials")
			}
			if *opts.ConnectTimeout != 10*time.Second || *opts.ServerSelectionTimeout != Timeout || *opts.MaxPoolSize != 50 || *opts.MinPoolSize != 0 || *opts.MaxConnIdleTime != time.Minute {
				t.Fatal("unexpected connection pool or timeout settings")
			}
		})
	}
}

func TestCredentialsRoundTrip(t *testing.T) {
	for _, authDB := range []string{"", "accounts"} {
		c := Config{Hosts: "localhost", Username: "user@:/%", Password: "p@(a)^b%:/?#$+", AuthDatabase: authDB}
		opts, err := c.Options()
		if err != nil {
			t.Fatal(err)
		}
		wantDB := authDB
		if wantDB == "" {
			wantDB = "admin"
		}
		if opts.Auth == nil || opts.Auth.Username != c.Username || opts.Auth.Password != c.Password || opts.Auth.AuthSource != wantDB {
			t.Fatal("credentials or authentication database changed during URI encoding")
		}
	}
}

func TestInvalidConnectionConfig(t *testing.T) {
	cases := []Config{
		{}, {Hosts: " "}, {Hosts: "a,,b"}, {Hosts: "mongodb://a:27017"},
		{Hosts: "a@b"}, {Hosts: "a/db"}, {Hosts: "a b"}, {Hosts: ":27017"},
		{Hosts: "a:abc"}, {Hosts: "a:0"}, {Hosts: "a:65536"},
		{Hosts: "a", Port: -1}, {Hosts: "a", Port: 65536},
		{Hosts: "[::1"}, {Hosts: "::1]"}, {Hosts: "[]"}, {Hosts: "[host]"}, {Hosts: "[host:name]:27017"},
		{Hosts: "a", Password: "private-test-password"},
	}
	for _, c := range cases {
		if _, err := c.Options(); err == nil {
			t.Errorf("invalid configuration accepted: hosts=%q port=%d", c.Hosts, c.Port)
		} else if c.Password != "" && strings.Contains(err.Error(), c.Password) {
			t.Fatal("password leaked in error")
		}
	}
}

// Opt-in and read-only: never creates or changes database contents.
func TestConnectIntegration(t *testing.T) {
	uri := os.Getenv("MONGO_TEST_URI")
	if uri == "" {
		t.Skip("set MONGO_TEST_URI to test a real MongoDB connection")
	}
	input := options.Client().ApplyURI(uri)
	if err := input.Validate(); err != nil {
		t.Fatal("invalid MONGO_TEST_URI")
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal("invalid MONGO_TEST_URI")
	}
	c := Config{Hosts: strings.Join(input.Hosts, ","), TLS: input.TLSConfig != nil, CA: os.Getenv("MONGO_TEST_CA_PEM"), Certificate: os.Getenv("MONGO_TEST_CERT_PEM"), Key: os.Getenv("MONGO_TEST_KEY_PEM")}
	if input.Auth != nil {
		c.Username = input.Auth.Username
		c.Password = input.Auth.Password
		c.AuthDatabase = input.Auth.AuthSource
		c.AuthMechanism = input.Auth.AuthMechanism
	}
	if input.ReplicaSet != nil {
		c.ReplicaSet = *input.ReplicaSet
	}
	client, err := Connect(c)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = client.Disconnect(ctx)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()
	var hello struct {
		SetName string   `bson:"setName"`
		Hosts   []string `bson:"hosts"`
	}
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
		t.Fatal(err)
	}
	if c.ReplicaSet != "" && hello.SetName != c.ReplicaSet {
		t.Fatal("connected to an unexpected replica set")
	}
	database := strings.TrimPrefix(parsed.Path, "/")
	if database == "" {
		database = "test"
	}
	names, err := client.Database(database).ListCollectionNames(ctx, bson.D{})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("authenticated connection: replica members=%d, database=%s, collections=%d", len(hello.Hosts), database, len(names))
}

func TestAuthMechanism(t *testing.T) {
	for _, mechanism := range []string{"", "SCRAM-SHA-1", "SCRAM-SHA-256"} {
		opts, err := (Config{Hosts: "localhost", Username: "user", Password: "p@ss%word", AuthMechanism: mechanism}).Options()
		if err != nil {
			t.Fatal(err)
		}
		if opts.Auth == nil || opts.Auth.AuthMechanism != mechanism || opts.Auth.Password != "p@ss%word" {
			t.Fatal("authentication configuration lost")
		}
	}
	for _, c := range []Config{
		{Hosts: "localhost", AuthMechanism: "SCRAM-SHA-1"},
		{Hosts: "localhost", Username: "user", AuthMechanism: "unsupported"},
	} {
		if _, err := c.Options(); err == nil {
			t.Fatal("invalid authentication configuration accepted")
		}
	}
}
