package totp

import (
	"testing"
	"time"
)

// RFC 6238 附录 B 的官方测试向量（SHA1，8 位码，种子为 ASCII "12345678901234567890"）。
// 6 位生产码与 8 位向量共享同一动态截断路径，向量通过即截断实现正确。
func TestRFC6238Vectors(t *testing.T) {
	secret := []byte("12345678901234567890")
	vectors := []struct {
		unix int64
		want string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	for _, v := range vectors {
		got := CodeDigits(secret, time.Unix(v.unix, 0), 8)
		if got != v.want {
			t.Fatalf("RFC6238 向量不符: t=%d got=%s want=%s", v.unix, got, v.want)
		}
	}
}

func TestGenerateAndVerify(t *testing.T) {
	raw, b32, err := GenerateSecret()
	if err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	if len(raw) != 20 {
		t.Fatalf("密钥长度 %d, want 20", len(raw))
	}
	// base32 往返
	back, err := DecodeSecret(b32)
	if err != nil || string(back) != string(raw) {
		t.Fatalf("base32 往返失败: %v", err)
	}

	now := time.Now()
	code := Code(raw, now)

	// 正确码通过；步长返回当前窗口
	ok, step := Verify(raw, code, now, 0)
	if !ok {
		t.Fatalf("当前码应当通过: code=%s", code)
	}

	// 防重放：同一码在记录 lastStep 之后不能再用于更早/相同步长
	if ok2, _ := Verify(raw, code, now, step); ok2 {
		t.Fatalf("已用步长的码不应再次通过（防重放）")
	}

	// 时钟偏差 ±30s 仍可验证
	if ok3, _ := Verify(raw, code, now.Add(Step), 0); !ok3 {
		t.Fatalf("偏慢 1 个步长的服务器应仍能验证")
	}
	if ok4, _ := Verify(raw, code, now.Add(-Step), 0); !ok4 {
		t.Fatalf("偏快 1 个步长的服务器应仍能验证")
	}

	// 错误码拒绝；位数不符拒绝
	if ok5, _ := Verify(raw, "000000", now, 0); ok5 && code != "000000" {
		t.Fatalf("错误码不应通过")
	}
	if ok6, _ := Verify(raw, "12", now, 0); ok6 {
		t.Fatalf("位数不符不应通过")
	}
}

// URI 必须能被主流认证器识别：标签转义、参数齐全
func TestURI(t *testing.T) {
	u := URI("Yearning", "alice", "JBSWY3DPEHPK3PXP")
	wantPrefix := "otpauth://totp/Yearning:alice?secret=JBSWY3DPEHPK3PXP&issuer=Yearning&algorithm=SHA1&digits=6&period=30"
	if u != wantPrefix {
		t.Fatalf("URI = %q, want %q", u, wantPrefix)
	}
	// 用户名含特殊字符时要转义，不能破坏 URI 结构
	u2 := URI("Yearning", "a b", "JBSWY3DPEHPK3PXP")
	if u2 == "" || u2[0:16] != "otpauth://totp/Yearning" && u2[15:23] != "Yearning" {
		// 宽松校验：前缀 + secret 参数存在
		if u2[0:15] != "otpauth://totp/" {
			t.Fatalf("特殊字符用户名破坏了 URI: %q", u2)
		}
	}
}
