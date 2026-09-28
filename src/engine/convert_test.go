package engine

import (
	"reflect"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestAuditRoleMongoFieldsAreMapped 核对 proto 里每个 mongo_* 字段都能被 AuditRoleToProto 填上。
//
// 挡的是「引擎加了开关、前端也加了开关，但主程序这一层没接线」这类静默失效：
// 规则集在库里是 JSON blob，但它先被反序列化进本包的 AuditRole 结构体，
// 再逐字段映射到 proto——没在这里声明的字段，从库到引擎的路上就被丢掉了，
// 而且不会报错（encoding/json 忽略未知键），表现只是「页面上勾了不生效」。
func TestAuditRoleMongoFieldsAreMapped(t *testing.T) {
	src := &AuditRole{}
	v := reflect.ValueOf(src).Elem()
	tp := v.Type()

	// 给本地结构体里所有 Mongo* 字段填非零值
	filled := 0
	for i := 0; i < tp.NumField(); i++ {
		f := tp.Field(i)
		if !strings.HasPrefix(f.Name, "Mongo") {
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
		t.Fatal("AuditRole 里没有任何 Mongo* 字段，结构体是不是被改坏了？")
	}

	got := AuditRoleToProto(src)
	if got == nil {
		t.Fatal("AuditRoleToProto 返回 nil")
	}

	// proto 侧每个 mongo_* 字段都必须非零，说明它确实被映射了
	fds := got.ProtoReflect().Descriptor().Fields()
	checked := 0
	for i := 0; i < fds.Len(); i++ {
		fd := fds.Get(i)
		if !strings.HasPrefix(strings.ToLower(string(fd.Name())), "mongo") {
			continue
		}
		checked++
		val := got.ProtoReflect().Get(fd)
		zero := false
		switch fd.Kind() {
		case protoreflect.BoolKind:
			zero = !val.Bool()
		case protoreflect.Int32Kind, protoreflect.Int64Kind:
			zero = val.Int() == 0
		case protoreflect.StringKind:
			zero = val.String() == ""
		}
		if zero {
			t.Errorf("proto 字段 %q 没被 AuditRoleToProto 填上：主程序 AuditRole 缺字段，或 convert.go 缺映射", fd.Name())
		}
	}
	if checked == 0 {
		t.Fatal("proto AuditRole 里没有任何 mongo_* 字段，生成物是不是过期了？")
	}
}
