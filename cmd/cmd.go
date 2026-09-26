package cmd

import (
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/lib/vars"
	"Yearning-go/src/model"
	"Yearning-go/src/service"
	"flag"
	"fmt"
	"os"
)

// Command 解析子命令。原先用 gookit/gcli，但实际只用到「子命令 + 两个字符串选项」，
// 标准库 flag 足够，且省掉 gcli 及其 gookit/color、goutil 等一串传递依赖。
//
//	Yearning install      [-c conf.toml]              安装及数据初始化
//	Yearning migrate      [-c conf.toml]              破坏性版本升级修复
//	Yearning reset_super  [-c conf.toml]              重置超级管理员密码
//	Yearning run          [-c conf.toml] [-p 8000]    启动
func Command() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	fmt.Println(LOGO)

	config := "conf.toml"
	port := "8000"

	switch os.Args[1] {
	case "install":
		parseConfig(os.Args[2:], &config)
		model.DBNew(config)
		service.Migrate()
	case "migrate":
		parseConfig(os.Args[2:], &config)
		model.DBNew(config)
		service.DelCol()
		service.MargeRuleGroup()
	case "reset_super":
		parseConfig(os.Args[2:], &config)
		model.DBNew(config)
		model.DB().Model(model.CoreAccount{}).Where("username =?", "admin").
			Updates(&model.CoreAccount{Password: factory.DjangoEncrypt("Yearning_admin", string(factory.GetRandom()))})
		fmt.Println(i18n.DefaultLang.Load(i18n.INFO_ADMIN_PASSWORD_RESET))
	case "run":
		fs := newFlagSet("run")
		fs.StringVar(&config, "config", "conf.toml", "配置文件路径")
		fs.StringVar(&config, "c", "conf.toml", "配置文件路径")
		fs.StringVar(&port, "port", "8000", "Yearning启动端口")
		fs.StringVar(&port, "p", "8000", "Yearning启动端口")
		_ = fs.Parse(os.Args[2:])
		model.DBNew(config)
		service.UpdateData()
		service.StartYearning(port)
	case "version", "-v", "-version", "--version":
		fmt.Printf("Yearning %s %s\n", vars.Version, vars.Kind)
	default:
		usage()
		os.Exit(1)
	}
}

// parseConfig 解析只需要配置文件路径的子命令（-c 与 --config 等价）
func parseConfig(args []string, config *string) {
	fs := newFlagSet("config")
	fs.StringVar(config, "config", "conf.toml", "配置文件路径")
	fs.StringVar(config, "c", "conf.toml", "配置文件路径")
	_ = fs.Parse(args)
}

// newFlagSet 用法与错误都写到 stdout，避免和日志流的顺序错乱
func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.SetOutput(os.Stdout)
	return fs
}

func usage() {
	fmt.Println(LOGO)
	fmt.Println("Yearning Mysql数据审核平台")
	fmt.Printf("版本: %s %s\n\n", vars.Version, vars.Kind)
	fmt.Println("用法:")
	fmt.Println("  Yearning install      [-c conf.toml]             安装及数据初始化")
	fmt.Println("  Yearning migrate      [-c conf.toml]             破坏性版本升级修复")
	fmt.Println("  Yearning reset_super  [-c conf.toml]             重置超级管理员密码")
	fmt.Println("  Yearning run          [-c conf.toml] [-p 8000]   启动")
	fmt.Println("  Yearning version                                 显示版本")
}
