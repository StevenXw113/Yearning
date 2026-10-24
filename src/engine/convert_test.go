package engine

import (
	"reflect"
	"strings"
	"testing"
)

// TestAuditRoleSelfRuleFieldsAreMapped 核对 proto 里每个自研规则字段
// （mongo_* / redis_* / doris_*）都能被 AuditRoleToAPI 填上。
//
// 挡的是「引擎加了开关、前端也加了开关，但主程序这一层没接线」这类静默失效：
// 规则集在库里是 JSON blob，但它先被反序列化进本包的 AuditRole 结构体，
// 再逐字段映射到 proto——没在这里声明的字段，从库到引擎的路上就被丢掉了，
// 而且不会报错（encoding/json 忽略未知键），表现只是「页面上勾了不生效」。
func TestAuditRoleSelfRuleFieldsAreMapped(t *testing.T) {
	src := &AuditRole{}
	v := reflect.ValueOf(src).Elem()
	tp := v.Type()

	// 给本地结构体里所有自研规则字段（Mongo* / Redis* / Doris*）填非零值
	filled := 0
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		if !isSelfRuleField(f.Name) {
			continue
		}
		switch f.Type.Kind() {
		case reflect.Bool:
			v.Field(i).SetBool(true)
		case reflect.Int:
			v.Field(i).SetInt(7)
		case reflect.String:
			v.Field(i).SetString("x")
		default:
			continue
		}
		filled++
	}
	if filled == 0 {
		t.Fatal("AuditRole 里没有任何自研规则字段，结构体是不是被改坏了？")
	}

	got := AuditRoleToAPI(src)
	if got == nil {
		t.Fatal("AuditRoleToAPI 返回 nil")
	}

	// 线缆侧每个自研规则字段都必须非零，说明它确实被映射了
	gv := reflect.ValueOf(got).Elem()
	gt := gv.Type()
	checked := 0
	for i := 0; i < gt.NumField(); i++ {
		f := gt.Field(i)
		if !isSelfRuleField(f.Name) {
			continue
		}
		checked++
		if gv.Field(i).IsZero() {
			t.Errorf("契约字段 %q 没被 AuditRoleToAPI 填上：主程序 AuditRole 缺字段，或 convert.go 缺映射", f.Name)
		}
	}
	if checked == 0 {
		t.Fatal("api.AuditRole 里没有任何自研规则字段，契约包是不是过期了？")
	}
}

// isSelfRuleField 主程序 AuditRole 结构体里自研规则的字段前缀。
func isSelfRuleField(name string) bool {
	return strings.HasPrefix(name, "Mongo") ||
		strings.HasPrefix(name, "Redis") ||
		strings.HasPrefix(name, "Doris")
}
