package audit

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/lib/vars"
	"Yearning-go/src/model"
	"bufio"
	"context"
	enginev1 "engine/gen/engine/v1"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// oscRunning 记录正在执行的 gh-ost 进程（key=work_id），供「中止」使用
var oscRunning sync.Map

// alterStmt 匹配单条 ALTER TABLE 语句，捕获表名与变更表达式。
// gh-ost 只支持 ALTER，且一次只迁移一张表，因此非单条 ALTER 的 DDL 一律交回引擎。
var alterStmt = regexp.MustCompile("(?is)^\\s*alter\\s+table\\s+`?([\\w$]+)`?\\s+(.+?)\\s*;?\\s*$")

// KillOSC 向正在执行的 gh-ost 发送 SIGTERM（gh-ost 收到后会自行清理影子表）。
// 返回是否确实存在运行中的 OSC。
func KillOSC(workId string) bool {
	v, ok := oscRunning.Load(workId)
	if !ok {
		return false
	}
	cmd, ok := v.(*exec.Cmd)
	if !ok || cmd.Process == nil {
		return false
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	return true
}

// resolveGhOst 返回 gh-ost 可执行文件路径：优先取配置，其次从 PATH 查找，都没有则返回空
func resolveGhOst() string {
	if p := strings.TrimSpace(model.C.General.GhOstPath); p != "" {
		return p
	}
	if p, err := exec.LookPath("gh-ost"); err == nil {
		return p
	}
	return ""
}

// oscPlan 判断该工单是否满足走 OSC 的条件：
// DDL + 单条 ALTER TABLE + 表行数估算达到阈值 + gh-ost 可用。
// reason 为不满足时的原因，用于日志——静默回退到引擎会让运维以为 OSC 生效了。
func oscPlan(order *model.CoreSqlOrder, source *model.CoreDataSource) (path, table, alter, reason string, ok bool) {
	if order.Type != vars.DDL {
		return "", "", "", "非 DDL 工单", false
	}
	if model.C.General.OscMinRows <= 0 {
		return "", "", "", "OSC 未启用(OscMinRows<=0)", false
	}
	m := alterStmt.FindStringSubmatch(order.SQL)
	if m == nil {
		return "", "", "", "SQL 不是单条 ALTER TABLE 语句", false
	}
	path = resolveGhOst()
	if path == "" {
		return "", "", "", "未找到 gh-ost 可执行文件，请配置 [General].GhOstPath", false
	}
	table, alter = m[1], strings.TrimSpace(m[2])
	// 多条语句会被上面的正则在同一条里吞掉，含分号即判为非单条，交回引擎
	if alter == "" || strings.Contains(alter, ";") {
		return "", "", "", "SQL 不是单条 ALTER TABLE 语句", false
	}
	if !tableReachesThreshold(order.DataBase, table, source) {
		return "", "", "", fmt.Sprintf("表 %s.%s 行数估算未达到阈值 %d", order.DataBase, table, model.C.General.OscMinRows), false
	}
	return path, table, alter, "", true
}

// tableReachesThreshold 用 information_schema 的行数估算（非精确 count，避免全表扫描）
// 判断表是否达到 OSC 阈值
func tableReachesThreshold(database, table string, source *model.CoreDataSource) bool {
	db, err := source.ConnectDB(database)
	if err != nil {
		return false
	}
	defer func() {
		_ = model.Close(db)
	}()
	var est int64
	if err := db.Raw("select coalesce(table_rows,0) as est from information_schema.tables where table_schema =? and table_name =?", database, table).Scan(&est).Error; err != nil {
		return false
	}
	return est >= model.C.General.OscMinRows
}

// checkAlter 用审核引擎对 ALTER 做静态审核。
// gh-ost 绕开了引擎的执行路径，规则校验不能一并绕过：存在错误级(critical/error)问题时返回 false，
// 交回引擎执行链路，由引擎按既有逻辑拒绝。
func checkAlter(order *model.CoreSqlOrder, source *model.CoreDataSource, password string, rule *engine.AuditRole) (ok bool, reason string) {
	client, conn, err := calls.NewClient()
	if err != nil {
		return false, fmt.Sprintf("引擎不可用: %v", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rep, err := client.Check(ctx, &enginev1.CheckRequest{
		Sql:    order.SQL,
		Schema: order.DataBase,
		Source: &enginev1.DataSource{
			Ip:       source.IP,
			Port:     int32(source.Port),
			Username: source.Username,
			Password: password,
			Ca:       source.CAFile,
			Cert:     source.Cert,
			Key:      source.KeyFile,
			Kind:     calls.DataSourceKind(source.DBType),
		},
		Lang: model.C.General.Lang,
		Rule: engine.AuditRoleToProto(rule),
	})
	if err != nil || rep == nil || !rep.Ok {
		return false, fmt.Sprintf("引擎审核失败: %v", calls.CombineReplyErr(rep, err))
	}
	for _, r := range rep.Records {
		// 注意：0 表示审核通过（见 engine/internal/server/check.go），仅 1=error 为阻断级，
		// 2=warning / 3=observe 不拦（proto 里 "critical=0 error=1" 的注释与实际实现不符）
		if r.Level == 1 {
			return false, fmt.Sprintf("引擎审核存在阻断级问题: %s", r.Error)
		}
	}
	return true, ""
}

// RunOSC 让 gh-ost 接管大表 ALTER 的在线变更，执行进度实时写入 core_sql_orders.osc_info，
// 工单详情页的「OSC」面板通过 WebSocket 读取该字段展示。
// handled=true 表示本次执行已由 gh-ost 接管（不再调用引擎 Exec），err 为其执行结果。
func RunOSC(order *model.CoreSqlOrder, source *model.CoreDataSource, password, actor string, rule *engine.AuditRole) (handled bool, err error) {
	path, table, alter, reason, ok := oscPlan(order, source)
	if !ok {
		if order.Type == vars.DDL {
			model.DefaultLogger.Infof("OSC 未接管(work_id=%s): %s，交由引擎执行", order.WorkId, reason)
		}
		return false, nil
	}
	if ok, reason := checkAlter(order, source, password, rule); !ok {
		model.DefaultLogger.Infof("OSC 未接管(work_id=%s): %s，交由引擎执行", order.WorkId, reason)
		return false, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 密码只能通过命令行参数传给 gh-ost（gh-ost 不支持从文件/环境变量读取密码），
	// 因此它会出现在同机 ps 输出中，与源库凭据的既有暴露面一致。
	argv := []string{
		"--host", source.IP,
		"--port", strconv.Itoa(source.Port),
		"--user", source.Username,
		"--password", password,
		"--database", order.DataBase,
		"--table", table,
		"--alter", alter,
		"--allow-on-master",            // 数据源通常直连主库，需显式允许
		"--assume-rbr",                 // 假定 binlog_format=ROW（非 ROW 时 gh-ost 无法迁移）
		"--ok-to-drop-table",           // 非交互：迁移完成后允许删除旧表
		"--initially-drop-ghost-table", // 清理上次中断残留的影子表，保证可重试
		"--initially-drop-old-table",
		"--execute",
	}
	// 环境差异（如账号无 performance_schema 权限）通过配置补充参数，避免默认关掉 gh-ost 自带的安全检查
	argv = append(argv, strings.Fields(model.C.General.GhOstArgs)...)
	cmd := exec.CommandContext(ctx, path, argv...)

	// gh-ost 的日志与进度行输出到 stdout/stderr 的版本不一，合并到同一管道读取
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	oscUpdate(order.WorkId, fmt.Sprintf("OSC(gh-ost) 已启动: %s.%s → %s", order.DataBase, table, alter))

	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		return true, err
	}
	oscRunning.Store(order.WorkId, cmd)
	defer oscRunning.Delete(order.WorkId)

	waitErr := make(chan error, 1)
	go func() {
		waitErr <- cmd.Wait()
		_ = pw.Close()
	}()

	var tail []string
	lastFlush := time.Time{}
	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		tail = append(tail, line)
		if len(tail) > 30 {
			tail = tail[1:]
		}
		// 进度行形如：Copy: 1234/5678 21.7%; Applied: 0; Backlog: 0/1000; ... 按秒节流落库
		if strings.HasPrefix(line, "Copy:") && time.Since(lastFlush) >= time.Second {
			lastFlush = time.Now()
			oscUpdate(order.WorkId, line)
		}
	}

	if werr := <-waitErr; werr != nil {
		// 失败时把输出尾部落到面板，便于直接看到 gh-ost 的报错原因
		oscUpdate(order.WorkId, strings.Join(tail, "\n"))
		return true, fmt.Errorf("OSC(gh-ost) 执行失败: %v\n%s", werr, strings.Join(tail, "\n"))
	}

	// 与引擎执行保持一致：写执行明细与操作流水（工单状态由调用方置为成功）
	model.DB().Create(&model.CoreSqlRecord{
		WorkId: order.WorkId,
		SQL:    order.SQL,
		State:  "已执行",
		Time:   time.Now().Format("2006-01-02 15:04"),
	})
	model.DB().Create(&model.CoreWorkflowDetail{
		WorkId:   order.WorkId,
		Username: actor,
		Time:     time.Now().Format("2006-01-02 15:04"),
		Action:   i18n.DefaultLang.Load(i18n.ORDER_EXECUTE_STATE),
	})
	oscUpdate(order.WorkId, "OSC 执行完成\n"+strings.Join(tail, "\n"))
	return true, nil
}

// oscUpdate 更新工单的 OSC 面板内容（工单详情的 osc 面板通过 WS 轮询该字段）
func oscUpdate(workId, text string) {
	model.DB().Model(&model.CoreSqlOrder{}).Where("work_id =?", workId).Update("osc_info", text)
}
