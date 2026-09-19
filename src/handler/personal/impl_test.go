package personal

import (
	"os"
	"testing"

	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"

	"github.com/cookieY/sqlx"
	_ "github.com/go-sql-driver/mysql"
)

// 脱敏字段的匹配必须与大小写、前后空格无关：配置写成 Phone / Email 也要生效。
func TestExcludeFieldContextIgnoresCase(t *testing.T) {
	m := &MultiSQLRunner{InsulateWordList: factory.MapOn(lowerList([]string{" Phone ", "EMAIL"}))}
	cases := map[string]bool{
		"phone":    true,
		"PHONE":    true,
		"Phone":    true,
		"email":    true,
		"EMAIL":    true,
		"id":       false,
		"phone_no": false,
	}
	for field, want := range cases {
		if got := m.excludeFieldContext(field); got != want {
			t.Errorf("excludeFieldContext(%q) = %v，期望 %v", field, got, want)
		}
	}
}

// 真实库端到端：脱敏字段无论什么类型都只返回占位符，非脱敏字段原样返回。
//
//	YEARING_TEST_DSN='demo:Demo_123456@tcp(10.10.10.30:3306)/demo?charset=utf8mb4&parseTime=true' \
//	  go test ./src/handler/personal/ -run TestRunMasksEveryType -v
func TestRunMasksEveryType(t *testing.T) {
	dsn := os.Getenv("YEARING_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 YEARING_TEST_DSN，跳过真实库脱敏验证")
	}
	i18n.MakeBuild(i18n.CN)
	masked := i18n.DefaultLang.Load(i18n.INFO_SENSITIVE_FIELD)
	if masked == "" {
		t.Fatal("脱敏占位文案为空，i18n 未初始化")
	}

	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	defer db.Close()

	// amount 是 float64、nothing 是 NULL、email 是字符串：三种不同类型都必须脱敏
	runner := &MultiSQLRunner{
		SQL:              "SELECT id, email, CAST(1.5 AS DOUBLE) AS amount, NULL AS nothing FROM users LIMIT 1",
		InsulateWordList: factory.MapOn(lowerList([]string{"Email", "AMOUNT", "nothing"})),
	}
	got, err := runner.Run(db, "demo")
	if err != nil {
		t.Fatalf("执行查询失败: %v", err)
	}
	if len(got.Data) == 0 {
		t.Fatal("没有返回数据，测试库应有 users 记录")
	}
	row := got.Data[0]
	for _, f := range []string{"email", "amount", "nothing"} {
		if v, ok := row[f]; !ok || v != masked {
			t.Errorf("字段 %s 未被脱敏: %#v", f, row[f])
		}
	}
	if v := row["id"]; v == masked {
		t.Errorf("未配置脱敏的 id 不该被替换: %#v", v)
	}
	t.Logf("脱敏结果: %#v", row)
}
