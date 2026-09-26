package enc

import (
	"encoding/hex"
	"testing"
)

// TestDeriveKeyMatchesPreviousImplementation 固化 HKDF 派生的输出。
// 期望值由旧实现（golang.org/x/crypto/hkdf + io.ReadFull）产出；换成标准库
// crypto/hkdf 后必须逐字节一致——否则存量数据源口令解不开、已签发的 JWT 全部失效。
func TestDeriveKeyMatchesPreviousImplementation(t *testing.T) {
	cases := []struct {
		master string
		info   string
		want   string
	}{
		{
			master: "master-key",
			info:   "yearning-jwt-hs256-v1",
			want:   "d9a1a51cd00aaf3d9b32c41dcfd4767a3c7c607546aeb7f4cb5b724756489c2a",
		},
		{
			master: "GlY3vGUnUJKUqAAziP9ls+LnKmAH7g48anbu5lLG284=",
			info:   "yearning-db-password-v1",
			want:   "108145b9950d9e75c7045312396171a6efd54c6c932f77c8d8c91db1a9312595",
		},
	}
	for _, c := range cases {
		got := hex.EncodeToString(DeriveKey(c.master, c.info, 32))
		if got != c.want {
			t.Errorf("DeriveKey(%q) = %s, want %s", c.info, got, c.want)
		}
	}
}
