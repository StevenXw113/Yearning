package dbengine

import "fmt"

// registry 按 db_type 注册引擎。init 里注册，key 与 model.DBType* 常量一致。
var registry = map[int]Engine{}

// register 注册一个引擎，重复注册直接 panic（与 mongocheck.Register 同理，
// 让配置错误在启动期暴露，而不是运行时踩到错误的引擎实现）。
func register(dbType int, e Engine) {
	if e == nil {
		panic("dbengine: register 的引擎为 nil")
	}
	if _, dup := registry[dbType]; dup {
		panic(fmt.Sprintf("dbengine: db_type %d 重复注册", dbType))
	}
	registry[dbType] = e
}

// For 按 db_type 取引擎；未注册的类型返回一个「拒绝一切」的占位实现。
func For(dbType int) Engine {
	if e, ok := registry[dbType]; ok {
		return e
	}
	return rejected{}
}
