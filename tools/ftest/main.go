// Command ftest 对一套已部署的 Yearning 做端到端功能测试。
//
// 走真实链路：主程序 HTTP/WS API → 审核引擎 gRPC → 目标库；断言结果直接读元数据库与目标库核对，
// 而不是只看接口返回码。适合每次发版后对目标环境跑一遍。
//
// 用法（仓库根目录）：
//
//	go run ./tools/ftest \
//	  -base http://127.0.0.1:8000 \
//	  -meta-dsn 'root:pwd@tcp(10.10.10.30:3306)/Yearning_go?charset=utf8mb4&parseTime=true' \
//	  -target-dsn 'demo:Demo_123456@tcp(10.10.10.30:3306)/demo?charset=utf8mb4&parseTime=true' \
//	  -source-id <数据源 source_id> -schema demo -table users -id-column id
//
// 前置条件：
//   - 主程序与引擎都在运行；
//   - 四个测试账号：管理员 / 申请人 / 审批人 / 无关人（默认 admin / dev1 / dba1 / readonly1）；
//   - 流程模板至少两级（提交级 + 审批级），审批级下标用 -approve-flag 指定（两级流程=1）；
//   - 目标库有一张可读写的测试表（默认 demo.users，主键 id，含 email 列用于校验脱敏与回滚）。
//
// 覆盖：认证与越权、工单全流程（立即/定时/人工执行、驳回、撤销、回滚语句）、
// 项目级工单（批量提交多条 SQL、批次查询、逐条审批后停等待执行、批量执行、失败即停、原子性）、
// 规则（级别灰度、自研规则、空载荷护栏、规则集增删与回滚）、查询链路（结果、脱敏、审计记录、导出标志）、
// 权限读写、上游版本检测。
//
// 数据：**测试产生的数据会保留**（工单文本以 FT- 开头，便于在界面上辨认与人工清理），
// 只把过程中改过的配置还原（规则集、数据源脱敏字段、查询导出开关）。
//
// 退出码：0=全部通过；1=有用例失败；2=参数/环境问题。
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/net/websocket"
)

var (
	base        = flag.String("base", "http://127.0.0.1:8000", "主程序地址")
	metaDSN     = flag.String("meta-dsn", "", "元数据库 DSN（读工单状态、改配置）")
	targetDSN   = flag.String("target-dsn", "", "目标库 DSN（校验 SQL 是否生效、回滚是否还原）")
	sourceID    = flag.String("source-id", "", "数据源 source_id")
	schema      = flag.String("schema", "demo", "目标库名")
	tableName   = flag.String("table", "users", "测试表名")
	idColumn    = flag.String("id-column", "id", "测试表主键列")
	idValue     = flag.Int("id-value", 1, "用于校验改动的行主键值")
	usersFlag   = flag.String("users", "admin:Yearning_admin,dev1:Dev_123456,dba1:Dba_123456,readonly1:Ro_123456", "账号，顺序固定：管理员,申请人,审批人,无关人（user:pass 逗号分隔）")
	approveFlag = flag.Int("approve-flag", 1, "审批级在流程模板中的下标（两级流程=1）")
)

const marker = "FT-"

var (
	metaDB  *sql.DB
	target  *sql.DB
	fails   []string
	pass    int
	skipped int
)

func check(id, name string, ok bool, detail string) {
	if ok {
		pass++
		fmt.Printf("%-8s %-44s PASS %s\n", id, name, detail)
		return
	}
	fails = append(fails, strings.TrimSpace(id+" "+name+" "+detail))
	fmt.Printf("%-8s %-44s FAIL %s\n", id, name, detail)
}

// skip 用于「依赖外部环境、当前不可用」的用例（例如上游 GitHub 不可达），不计入失败
func skip(id, name, detail string) {
	skipped++
	fmt.Printf("%-8s %-44s SKIP %s\n", id, name, detail)
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "ftest:", msg)
	os.Exit(2)
}

func call(token, path string, body any, method ...string) map[string]any {
	m := "POST"
	if len(method) > 0 {
		m = method[0]
	}
	var payload []byte
	if body == nil {
		payload = []byte("{}")
	} else {
		payload, _ = json.Marshal(body)
	}
	req, err := http.NewRequest(m, *base+path, strings.NewReader(string(payload)))
	if err != nil {
		return map[string]any{"code": -1.0, "text": err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{"code": -1.0, "text": err.Error()}
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out
}

func codeOf(r map[string]any) float64 {
	c, _ := r["code"].(float64)
	return c
}

func textOf(r map[string]any) string {
	t, _ := r["text"].(string)
	return t
}

func brief(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 140 {
		return string(b)[:140]
	}
	return string(b)
}

func login(u, p string) string {
	r := call("", "/login", map[string]string{"username": u, "password": p})
	pl, _ := r["payload"].(map[string]any)
	t, _ := pl["token"].(string)
	return t
}

func submit(token, text, sql, delay string, backup int) {
	call(token, "/api/v2/common/post", map[string]any{
		"source_id": *sourceID, "data_base": *schema, "table": *tableName, "text": text,
		"type": 1, "backup": backup, "delay": delay, "sql": sql,
	})
}

func orderOf(text string) (string, int, string) {
	var wid, delay string
	var status int
	_ = metaDB.QueryRow("SELECT work_id, `status`, `delay` FROM core_sql_orders WHERE `text` = ? ORDER BY id DESC LIMIT 1", text).
		Scan(&wid, &status, &delay)
	return wid, status, delay
}

func statusOf(wid string) int {
	var s int
	_ = metaDB.QueryRow("SELECT `status` FROM core_sql_orders WHERE work_id = ?", wid).Scan(&s)
	return s
}

func checkSQL(token, sql string) (float64, string, string) {
	r := call(token, "/api/v2/fetch/test", map[string]any{"sql": sql, "source_id": *sourceID, "data_base": *schema, "table": ""}, "PUT")
	pl, _ := r["payload"].([]any)
	if len(pl) == 0 {
		return -1, "", textOf(r)
	}
	rec, _ := pl[0].(map[string]any)
	lv, _ := rec["level"].(float64)
	st, _ := rec["status"].(string)
	er, _ := rec["error"].(string)
	return lv, st, er
}

func cellValue() string {
	var v string
	_ = target.QueryRow("SELECT email FROM "+*tableName+" WHERE "+*idColumn+" = ?", *idValue).Scan(&v)
	return v
}

// wsQuery 走查询 WebSocket 链路取一批数据（查询审核关闭时，后端要求存在已批准的查询工单）
func wsQuery(token, sql string) (rows []map[string]any, export bool, err error) {
	call(token, "/api/v2/query/post", map[string]any{"text": marker + "查询", "export": 0, "source_id": *sourceID})
	ws, derr := websocket.Dial("ws://"+strings.TrimPrefix(*base, "http://")+"/api/v2/query/results?source_id="+*sourceID, token, *base)
	if derr != nil {
		return nil, false, derr
	}
	defer ws.Close()
	payload, _ := msgpack.Marshal(map[string]any{"type": 4, "sql": sql, "schema": *schema})
	if err = websocket.Message.Send(ws, payload); err != nil {
		return nil, false, err
	}
	_ = ws.SetReadDeadline(time.Now().Add(25 * time.Second))
	var raw []byte
	if err = websocket.Message.Receive(ws, &raw); err != nil {
		return nil, false, err
	}
	var reply struct {
		Error   string `msgpack:"error"`
		Export  bool   `msgpack:"export"`
		Results []struct {
			Data []map[string]any `msgpack:"data"`
		} `msgpack:"results"`
	}
	if err = msgpack.Unmarshal(raw, &reply); err != nil {
		return nil, false, err
	}
	if reply.Error != "" {
		return nil, reply.Export, errors.New(reply.Error)
	}
	if len(reply.Results) > 0 {
		rows = reply.Results[0].Data
	}
	return rows, reply.Export, nil
}

// batchSubmit 提交项目级工单（批量）：items 中每条明细生成一个独立子工单，共享批次号。
func batchSubmit(token, text string, tp, backup int, delay string, items []map[string]any) map[string]any {
	return call(token, "/api/v2/common/batch", map[string]any{
		"type": tp, "backup": backup, "delay": delay, "text": text, "items": items,
	})
}

// batchWorkIds 按提交顺序返回一个批次下的子工单号。
func batchWorkIds(bid string) []string {
	rows, err := metaDB.Query("SELECT work_id FROM core_sql_orders WHERE batch_id = ? ORDER BY id", bid)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var w string
		_ = rows.Scan(&w)
		out = append(out, w)
	}
	return out
}

func batchCount(query string, args ...any) int {
	var n int
	_ = metaDB.QueryRow(query, args...).Scan(&n)
	return n
}

func emailOf(id int) string {
	var v string
	_ = target.QueryRow("SELECT email FROM "+*tableName+" WHERE "+*idColumn+" = ?", id).Scan(&v)
	return v
}

// wsAsk 在已建立的工单列表 WS 连接上取一页，返回行与总页数（按项目分页时即项目组数）。
func wsAsk(ws *websocket.Conn, current, pageSize int) ([]map[string]any, int64) {
	body, _ := json.Marshal(map[string]any{
		"current": current, "pageSize": pageSize,
		"expr": map[string]any{"status": 8, "type": 2, "text": "", "username": ""},
	})
	if websocket.Message.Send(ws, string(body)) != nil {
		return nil, 0
	}
	var raw []byte
	if websocket.Message.Receive(ws, &raw) != nil {
		return nil, 0
	}
	var reply struct {
		Payload struct {
			Data []map[string]any `json:"data"`
			Page int64            `json:"page"`
		} `json:"payload"`
	}
	_ = json.Unmarshal(raw, &reply)
	return reply.Payload.Data, reply.Payload.Page
}

// auditRows 取审批列表一页
func auditRows(token string, current, pageSize int) []map[string]any {
	ws, err := websocket.Dial("ws://"+strings.TrimPrefix(*base, "http://")+"/api/v2/audit/order/list", token, *base)
	if err != nil {
		return nil
	}
	defer ws.Close()
	rows, _ := wsAsk(ws, current, pageSize)
	return rows
}

// listBatchPages 拉取工单列表前 pages 页，返回 batch_id -> 出现过的页码集合。
// 用于验证「同一项目(批次)的子工单不会被分页拆到不同页各显示一组」。
func listBatchPages(token, path string, pageSize, pages int) map[string]map[int]bool {
	out := map[string]map[int]bool{}
	ws, err := websocket.Dial("ws://"+strings.TrimPrefix(*base, "http://")+path, token, *base)
	if err != nil {
		return out
	}
	defer ws.Close()
	for p := 1; p <= pages; p++ {
		rows, _ := wsAsk(ws, p, pageSize)
		for _, row := range rows {
			bid, _ := row["batch_id"].(string)
			if bid == "" {
				continue
			}
			if out[bid] == nil {
				out[bid] = map[int]bool{}
			}
			out[bid][p] = true
		}
	}
	return out
}

// splitBatches 统计被分页拆到多页的批次数（应为 0）
func splitBatches(seen map[string]map[int]bool) int {
	n := 0
	for _, pages := range seen {
		if len(pages) > 1 {
			n++
		}
	}
	return n
}

// suffixNum 取工单号「项目号-序号」里的序号；非项目子工单返回 0
func suffixNum(wid string) int {
	i := strings.LastIndex(wid, "-")
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(wid[i+1:])
	return n
}

func main() {
	flag.Parse()
	if *metaDSN == "" || *targetDSN == "" || *sourceID == "" {
		flag.Usage()
		fatal("必须提供 -meta-dsn / -target-dsn / -source-id")
	}
	var err error
	if metaDB, err = sql.Open("mysql", *metaDSN); err != nil {
		fatal(err.Error())
	}
	if target, err = sql.Open("mysql", *targetDSN); err != nil {
		fatal(err.Error())
	}
	defer metaDB.Close()
	defer target.Close()

	accs := strings.Split(*usersFlag, ",")
	if len(accs) != 4 {
		fatal("-users 需要 4 个账号：管理员,申请人,审批人,无关人")
	}
	cred := func(i int) (string, string) {
		kv := strings.SplitN(accs[i], ":", 2)
		if len(kv) != 2 {
			fatal("账号格式应为 user:pass")
		}
		return kv[0], kv[1]
	}
	adminTok := login(cred(0))
	devTok := login(cred(1))
	dbaTok := login(cred(2))
	roTok := login(cred(3))
	if adminTok == "" || devTok == "" || dbaTok == "" || roTok == "" {
		fatal("登录失败：检查 -users 与主程序是否在跑")
	}

	// 基线：目标表首行的原值、数据源脱敏字段原值、全局规则集原值
	var origInsulate string
	_ = metaDB.QueryRow("SELECT insulate_word_list FROM core_data_sources WHERE source_id = ?", *sourceID).Scan(&origInsulate)
	origCell := cellValue()
	origRule := call(adminTok, "/api/v2/manage/roles/global", nil)
	settings := call(adminTok, "/api/v2/manage/setting", nil, "GET")["payload"].(map[string]any)
	origExport, _ := settings["other"].(map[string]any)["export"].(bool)

	fmt.Printf("ftest 目标 %s（数据源 %s，表 %s.%s）\n\n", *base, *sourceID, *schema, *tableName)

	fmt.Println("=== 组 1 认证与权限 ===")
	check("T1.1", "四个测试账号登录成功", adminTok != "" && devTok != "" && dbaTok != "" && roTok != "", "")
	r := call(devTok, "/api/v2/manage/roles/global_updated", origRule["payload"])
	check("T1.2", "申请人保存全局规则集被拒（越权）", codeOf(r) != 1200, fmt.Sprintf("code=%v", r["code"]))
	r = call(roTok, "/api/v2/audit/order/state", map[string]any{"tp": "execute", "work_id": "not-exist"})
	check("T1.3", "无关人执行他人工单被拒", codeOf(r) != 1200, fmt.Sprintf("code=%v text=%s", r["code"], textOf(r)))

	// 超级管理员拥有所有权限：不受「相关人」与权限组约束
	submit(devTok, marker+"管理员执行", fmt.Sprintf("UPDATE %s SET email = email WHERE %s = %d;", *tableName, *idColumn, *idValue), "manual", 0)
	widAdm, _, _ := orderOf(marker + "管理员执行")
	call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "agree", "work_id": widAdm, "flag": *approveFlag})
	waitAdm := statusOf(widAdm) == 5
	r = call(adminTok, "/api/v2/audit/order/state", map[string]any{"tp": "execute", "work_id": widAdm})
	check("T1.4", "超级管理员可执行他人工单（不受「相关人」限制）",
		waitAdm && codeOf(r) == 1200 && statusOf(widAdm) == 1,
		fmt.Sprintf("wait=%v code=%v status=%d", waitAdm, r["code"], statusOf(widAdm)))

	adminRows := auditRows(adminTok, 1, 50)
	notMine := 0
	for _, row := range adminRows {
		if u, _ := row["username"].(string); u != "admin" {
			notMine++
		}
	}
	check("T1.5", "超级管理员审批列表可见全部工单（不按相关人过滤）",
		len(adminRows) > 0 && notMine > 0, fmt.Sprintf("rows=%d 非admin提交=%d", len(adminRows), notMine))

	fmt.Println("\n=== 组 2 工单主流程 ===")
	noop := fmt.Sprintf("UPDATE %s SET email = email WHERE %s = %d;", *tableName, *idColumn, *idValue)
	change := fmt.Sprintf("UPDATE %s SET email = 'ft-rollback@demo.com' WHERE %s = %d;", *tableName, *idColumn, *idValue)
	submit(devTok, marker+"立即", noop, "none", 0)
	submit(devTok, marker+"定时", noop, "2026-12-31 03:00", 0)
	submit(devTok, marker+"人工", noop, "manual", 0)
	submit(devTok, marker+"驳回", noop, "none", 0)
	submit(devTok, marker+"撤销", noop, "none", 0)
	submit(devTok, marker+"回滚", change, "none", 1)

	widNow, st, _ := orderOf(marker + "立即")
	var widNowID int64
	_ = metaDB.QueryRow("SELECT id FROM core_sql_orders WHERE work_id = ?", widNow).Scan(&widNowID)
	check("T2.1", "工单号 = 该工单的自增 id（唯一）", widNow == fmt.Sprint(widNowID),
		fmt.Sprintf("work_id=%s id=%d", widNow, widNowID))
	check("T2.2", "提交后状态=2（审核中）", st == 2, fmt.Sprintf("status=%d", st))

	lv, _, _ := checkSQL(dbaTok, noop)
	check("T2.3", "SQL 检测通过（level=0）", lv == 0, fmt.Sprintf("level=%v", lv))

	call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "agree", "work_id": widNow, "flag": *approveFlag})
	check("T2.4", "立即执行：审批后 status=1（已执行）", statusOf(widNow) == 1, fmt.Sprintf("status=%d", statusOf(widNow)))

	widT, _, delayT := orderOf(marker + "定时")
	call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "agree", "work_id": widT, "flag": *approveFlag})
	check("T2.5", "定时执行：审批后 status=5 且保留 delay 时间",
		statusOf(widT) == 5 && strings.Contains(delayT, "-"), fmt.Sprintf("status=%d delay=%s", statusOf(widT), delayT))

	widM, _, delayM := orderOf(marker + "人工")
	call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "agree", "work_id": widM, "flag": *approveFlag})
	waitOK := statusOf(widM) == 5 && delayM == "manual"
	r = call(roTok, "/api/v2/audit/order/state", map[string]any{"tp": "execute", "work_id": widM})
	rejOK := codeOf(r) != 1200 && strings.Contains(textOf(r), "相关人")
	call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "execute", "work_id": widM})
	check("T2.6", "人工执行：等待→相关人执行成功、无关人被拒",
		waitOK && rejOK && statusOf(widM) == 1, fmt.Sprintf("wait=%v reject=%v final=%d", waitOK, rejOK, statusOf(widM)))

	widR, _, _ := orderOf(marker + "驳回")
	r = call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "reject", "work_id": widR, "flag": 0, "text": "层级不对"})
	msg := textOf(r)
	check("T2.7", "驳回：层级不对时给出准确原因（非权限文案）",
		codeOf(r) != 1200 && strings.Contains(msg, "审核人") && !strings.Contains(msg, "%s"), fmt.Sprintf("text=%s", msg))
	call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "reject", "work_id": widR, "flag": *approveFlag, "text": "功能测试驳回"})
	check("T2.7b", "驳回：本阶段审核人驳回成功 status=0", statusOf(widR) == 0, fmt.Sprintf("status=%d", statusOf(widR)))

	widU, _, _ := orderOf(marker + "撤销")
	call(devTok, "/api/v2/audit/order/state", map[string]any{"tp": "undo", "work_id": widU})
	check("T2.8", "申请人撤销后 status=6", statusOf(widU) == 6, fmt.Sprintf("status=%d", statusOf(widU)))

	widB, _, _ := orderOf(marker + "回滚")
	call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "agree", "work_id": widB, "flag": *approveFlag})
	after := cellValue()
	var rbSQL string
	_ = metaDB.QueryRow("SELECT `sql` FROM core_rollbacks WHERE work_id = ? ORDER BY id DESC LIMIT 1", widB).Scan(&rbSQL)
	if rbSQL != "" {
		_, _ = target.Exec(rbSQL)
	}
	check("T2.9", "回滚：改动生效 → 回滚语句把数据还原",
		statusOf(widB) == 1 && after == "ft-rollback@demo.com" && cellValue() == origCell,
		fmt.Sprintf("after=%s rollback=%q restored=%s", after, rbSQL, cellValue()))

	fmt.Println("\n=== 组 3 规则 ===")
	copyMap := func(src any) map[string]any {
		out := map[string]any{}
		if m, ok := src.(map[string]any); ok {
			for k, v := range m {
				out[k] = v
			}
		}
		return out
	}
	warn := copyMap(origRule["payload"])
	warn["DMLWhere"] = true
	warn["RuleLevel"] = map[string]string{"DMLWhere": "warn"}
	call(adminTok, "/api/v2/manage/roles/global_updated", warn)
	lv, st2, _ := checkSQL(adminTok, "DELETE FROM "+*tableName)
	check("T3.1", "规则级别 warn：命中只提示不拦", lv == 2 && st2 == "警告", fmt.Sprintf("level=%v status=%s", lv, st2))

	trunc := copyMap(origRule["payload"])
	trunc["DDLForbidTruncate"] = true
	call(adminTok, "/api/v2/manage/roles/global_updated", trunc)
	lv, _, _ = checkSQL(adminTok, "TRUNCATE TABLE "+*tableName)
	check("T3.2", "自研规则（禁止 TRUNCATE）生效", lv == 1, fmt.Sprintf("level=%v", lv))

	r = call(adminTok, "/api/v2/manage/roles/global_updated", map[string]any{})
	check("T3.3", "空载荷规则集被护栏拒绝", codeOf(r) == 5555, fmt.Sprintf("code=%v", r["code"]))

	call(adminTok, "/api/v2/manage/roles/global_updated", origRule["payload"]) // 还原规则集
	call(adminTok, "/api/v2/manage/roles/add", map[string]any{"desc": marker + "临时集", "audit_role": origRule["payload"]})
	var rid float64
	if pl, ok := call(adminTok, "/api/v2/manage/roles/list", nil)["payload"].([]any); ok {
		for _, it := range pl {
			if m, ok := it.(map[string]any); ok && m["desc"] == marker+"临时集" {
				rid, _ = m["id"].(float64)
			}
		}
	}
	r = call(adminTok, "/api/v2/manage/roles/delete", map[string]any{"id": int(rid)})
	check("T3.4", "规则集新建后可删除", rid > 0 && codeOf(r) == 1200, fmt.Sprintf("id=%v code=%v", rid, r["code"]))
	rbCode := -1.0
	if hl, ok := call(adminTok, "/api/v2/manage/roles/history", map[string]any{"rule_id": int(rid)})["payload"].([]any); ok && len(hl) > 0 {
		first, _ := hl[0].(map[string]any)
		rbCode = codeOf(call(adminTok, "/api/v2/manage/roles/rollback", map[string]any{"id": int(first["id"].(float64))}))
	}
	check("T3.5", "删除后可从变更历史回滚恢复", rbCode == 1200, fmt.Sprintf("code=%v", rbCode))

	fmt.Println("\n=== 组 4 查询链路 ===")
	rows, export0, err := wsQuery(adminTok, "SELECT id, name, email FROM "+*tableName+" WHERE "+*idColumn+" = "+fmt.Sprint(*idValue))
	check("T4.1", "查询返回结果行（WS）", err == nil && len(rows) > 0, fmt.Sprintf("rows=%d err=%v", len(rows), err))
	check("T4.2", "导出关闭时 export=false", !export0, fmt.Sprintf("export=%v", export0))

	var qrec int
	_ = metaDB.QueryRow("SELECT count(*) FROM core_query_records WHERE `sql` LIKE 'SELECT id, name, email%'").Scan(&qrec)
	check("T4.3", "查询审计记录已落库（同步写）", qrec > 0, fmt.Sprintf("rows=%d", qrec))

	// 脱敏：把 email 配成脱敏字段后，结果里该列应变成占位符
	if _, err := metaDB.Exec("UPDATE core_data_sources SET insulate_word_list = ? WHERE source_id = ?", "Email,NAME", *sourceID); err != nil {
		check("T4.4", "查询结果脱敏生效", false, err.Error())
	} else {
		rows, _, err = wsQuery(adminTok, "SELECT id, name, email FROM "+*tableName+" WHERE "+*idColumn+" = "+fmt.Sprint(*idValue))
		masked := false
		for _, row := range rows {
			if v, ok := row["email"].(string); ok && strings.Contains(v, "脱敏") {
				masked = true
			}
		}
		check("T4.4", "查询结果脱敏生效（email 列被掩码）", err == nil && masked, fmt.Sprintf("rows=%d sample=%v", len(rows), brief(rows)))
		_, _ = metaDB.Exec("UPDATE core_data_sources SET insulate_word_list = ? WHERE source_id = ?", origInsulate, *sourceID)
	}

	// 导出开关：打开后响应应带 export=true
	if other, ok := settings["other"].(map[string]any); ok {
		other["export"] = true
		call(adminTok, "/api/v2/manage/setting", map[string]any{"message": settings["message"], "other": other, "ai": settings["ai"]})
		_, export1, err := wsQuery(adminTok, "SELECT 1")
		check("T4.5", "打开导出开关后 export=true", err == nil && export1, fmt.Sprintf("export=%v err=%v", export1, err))
		other["export"] = origExport
		call(adminTok, "/api/v2/manage/setting", map[string]any{"message": settings["message"], "other": other, "ai": settings["ai"]})
	}

	// 查询工单编号同样基于自增 id
	var qID int64
	var qWid string
	_ = metaDB.QueryRow("SELECT id, work_id FROM core_query_orders ORDER BY id DESC LIMIT 1").Scan(&qID, &qWid)
	check("T4.6", "查询工单编号 = 自增 id", qWid != "" && qWid == fmt.Sprint(qID),
		fmt.Sprintf("work_id=%s id=%d", qWid, qID))

	fmt.Println("\n=== 组 5 权限管理 ===")
	applicant, _ := cred(1)
	r = call(adminTok, "/api/v2/fetch/groups?user="+applicant, nil, "GET")
	pay, _ := r["payload"].(map[string]any)
	groups, _ := pay["groups"].([]any)
	own, _ := pay["own"].([]any)
	check("T5.1", "读取用户权限组（own + 可选组）", len(groups) > 0 && own != nil,
		fmt.Sprintf("groups=%d own=%d", len(groups), len(own)))
	r = call(adminTok, "/api/v2/manage/user?tp=policy", map[string]any{"username": applicant, "group": own})
	check("T5.2", "写用户权限组（原值回写）", codeOf(r) == 1200, fmt.Sprintf("code=%v", r["code"]))

	fmt.Println("\n=== 组 6 运维接口 ===")
	r = call(adminTok, "/api/v2/manage/roles/upstream", nil)
	if strings.Contains(textOf(r), "无法访问上游仓库") {
		// 上游检测要访问 GitHub，网络不通属环境问题：跳过而不是判失败
		skip("T6.1", "上游版本检测（页面按钮接口）", "上游不可达："+textOf(r))
	} else {
		up, _ := r["payload"].(map[string]any)
		check("T6.1", "上游版本检测（页面按钮接口）", codeOf(r) == 1200 && up["latest"] != nil, brief(up))
	}

	fmt.Println("\n=== 组 7 项目级工单（批量，多条 SQL） ===")
	batchItem := func(sql string) map[string]any {
		return map[string]any{"source_id": *sourceID, "data_base": *schema, "sql": sql}
	}
	uniq := func(tag string) string { return fmt.Sprintf("%s%s-%d", marker, tag, time.Now().UnixNano()) }
	// 本组会改动 users 的 id=1 / id=3 两行的 email，跑完还原，避免污染其它用例
	origB1, origB3 := emailOf(1), emailOf(3)

	// 一个批次内 3 条明细（多条 SQL），其中第 2 条明细自身含 2 条语句
	batchText := uniq("项目工单")
	r = batchSubmit(devTok, batchText, 1, 1, "none", []map[string]any{
		batchItem(fmt.Sprintf("UPDATE %s SET email = 'ft-batch-1@demo.com' WHERE %s = 1;", *tableName, *idColumn)),
		batchItem(fmt.Sprintf("UPDATE %s SET email = 'ft-batch-2@demo.com' WHERE %s = 3; UPDATE %s SET name = name WHERE %s = 3;", *tableName, *idColumn, *tableName, *idColumn)),
		batchItem(fmt.Sprintf("UPDATE %s SET email = 'ft-batch-3@demo.com' WHERE %s = 1;", *tableName, *idColumn)),
	})
	bid, _ := r["payload"].(string)
	// 项目号 = 首条子工单的自增 id；子工单编号 = 项目号-1、项目号-2…（按提交顺序）
	var firstID int64
	_ = metaDB.QueryRow("SELECT id FROM core_sql_orders WHERE batch_id = ? ORDER BY id LIMIT 1", bid).Scan(&firstID)
	numsOK := fmt.Sprint(firstID) == bid
	for i, w := range batchWorkIds(bid) {
		if w != fmt.Sprintf("%s-%d", bid, i+1) {
			numsOK = false
		}
	}
	check("T7.1", "项目号=首条子工单自增 id，子工单号=项目号-序号",
		codeOf(r) == 1200 && numsOK,
		fmt.Sprintf("code=%v batch=%s children=%v", r["code"], bid, batchWorkIds(bid)))

	total := batchCount("SELECT COUNT(*) FROM core_sql_orders WHERE batch_id = ?", bid)
	distinctWid := batchCount("SELECT COUNT(DISTINCT work_id) FROM core_sql_orders WHERE batch_id = ?", bid)
	auditing := batchCount("SELECT COUNT(*) FROM core_sql_orders WHERE batch_id = ? AND `status` = 2", bid)
	check("T7.2", "多条明细各生成独立子工单并共享同一批次号",
		total == 3 && distinctWid == 3 && auditing == 3,
		fmt.Sprintf("total=%d distinct_wid=%d auditing=%d", total, distinctWid, auditing))

	qr := call(devTok, "/api/v2/common/batch?batch_id="+bid, nil, "GET")
	qpl, _ := qr["payload"].([]any)
	check("T7.3", "批次查询接口返回全部子工单", codeOf(qr) == 1200 && len(qpl) == 3,
		fmt.Sprintf("code=%v rows=%d", qr["code"], len(qpl)))

	wids := batchWorkIds(bid)
	// 项目级展示、逐条审核：审批一条只影响该条，其余仍在审核中
	if len(wids) > 0 {
		call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "agree", "work_id": wids[0], "flag": *approveFlag})
	}
	oneDone := batchCount("SELECT COUNT(*) FROM core_sql_orders WHERE batch_id = ? AND `status` = 5", bid)
	stillAuditing := batchCount("SELECT COUNT(*) FROM core_sql_orders WHERE batch_id = ? AND `status` = 2", bid)
	check("T7.4", "逐条审核：审批一条只通过该条，其余仍在审核中",
		oneDone == 1 && stillAuditing == 2, fmt.Sprintf("done=%d auditing=%d", oneDone, stillAuditing))

	// 分页单位是项目：同一批次的子工单不会被拆到不同页各显示一组
	spread := listBatchPages(dbaTok, "/api/v2/audit/order/list", 3, 6)
	check("T7.4b", "审批列表按项目分页：同批次子工单不跨页",
		len(spread) > 0 && splitBatches(spread) == 0,
		fmt.Sprintf("batches=%d split=%d", len(spread), splitBatches(spread)))

	for _, w := range wids[1:] {
		call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "agree", "work_id": w, "flag": *approveFlag})
	}
	wait5 := batchCount("SELECT COUNT(*) FROM core_sql_orders WHERE batch_id = ? AND `status` = 5", bid)
	check("T7.4c", "其余子工单逐条审批后都停在 status=5（不自动执行）", wait5 == 3,
		fmt.Sprintf("status5=%d", wait5))

	r = call(roTok, "/api/v2/audit/order/batch", map[string]any{"batch_id": bid})
	check("T7.5", "无关人批量执行被拒且工单状态不变",
		codeOf(r) == 5555 && batchCount("SELECT COUNT(*) FROM core_sql_orders WHERE batch_id = ? AND `status` = 5", bid) == 3,
		fmt.Sprintf("code=%v text=%s", r["code"], textOf(r)))

	r = call(dbaTok, "/api/v2/audit/order/batch", map[string]any{"batch_id": bid})
	res, _ := r["payload"].([]any)
	allOK := len(res) == 3
	for _, it := range res {
		if m, ok := it.(map[string]any); ok {
			if okv, _ := m["ok"].(bool); !okv {
				allOK = false
			}
		}
	}
	executed := batchCount("SELECT COUNT(*) FROM core_sql_orders WHERE batch_id = ? AND `status` = 1", bid)
	check("T7.6", "批量执行：3 条子工单全部成功、status=1",
		codeOf(r) == 1200 && allOK && executed == 3,
		fmt.Sprintf("code=%v results=%s executed=%d", r["code"], brief(res), executed))

	check("T7.7", "多条 SQL 按提交顺序落到目标库（含单工单内多语句）",
		emailOf(1) == "ft-batch-3@demo.com" && emailOf(3) == "ft-batch-2@demo.com",
		fmt.Sprintf("id1=%s id3=%s", emailOf(1), emailOf(3)))

	rbWids := -1
	if len(wids) == 3 {
		rbWids = batchCount("SELECT COUNT(DISTINCT work_id) FROM core_rollbacks WHERE work_id IN (?, ?, ?)", wids[0], wids[1], wids[2])
	}
	check("T7.8", "开启备份时每条子工单都落下回滚语句", rbWids == 3, fmt.Sprintf("with_rollback=%d", rbWids))

	// 失败即停：第 2 条执行失败后，第 3 条不再执行（后续 SQL 可能依赖前面的变更）
	failText := uniq("失败即停")
	r = batchSubmit(devTok, failText, 1, 0, "none", []map[string]any{
		batchItem(fmt.Sprintf("UPDATE %s SET email = 'ft-batchfail-1@demo.com' WHERE %s = 1;", *tableName, *idColumn)),
		batchItem(fmt.Sprintf("UPDATE demo.yrn_no_such_table_zzz SET v = 1 WHERE %s = 1;", *idColumn)),
		batchItem(fmt.Sprintf("UPDATE %s SET email = 'ft-batchfail-3@demo.com' WHERE %s = 3;", *tableName, *idColumn)),
	})
	fbid, _ := r["payload"].(string)
	for _, w := range batchWorkIds(fbid) {
		call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "agree", "work_id": w, "flag": *approveFlag})
	}
	r = call(dbaTok, "/api/v2/audit/order/batch", map[string]any{"batch_id": fbid})
	fres, _ := r["payload"].([]any)
	firstOK, secondFail := false, false
	if len(fres) >= 2 {
		m0, _ := fres[0].(map[string]any)
		m1, _ := fres[1].(map[string]any)
		firstOK, _ = m0["ok"].(bool)
		ok1, _ := m1["ok"].(bool)
		secondFail = !ok1
	}
	fWait := batchCount("SELECT COUNT(*) FROM core_sql_orders WHERE batch_id = ? AND `status` = 5", fbid)
	check("T7.9", "失败即停：失败项之后不再执行、未执行的保持 status=5",
		len(fres) == 2 && firstOK && secondFail && fWait == 2 && emailOf(3) == "ft-batch-2@demo.com",
		fmt.Sprintf("results=%d wait5=%d id3=%s", len(fres), fWait, emailOf(3)))

	// 原子性：任一明细缺字段则整批拒绝，不落库
	atomText := uniq("批量原子")
	r = batchSubmit(devTok, atomText, 1, 0, "none", []map[string]any{
		batchItem(noop),
		{"source_id": *sourceID, "data_base": "", "sql": noop},
	})
	check("T7.10", "明细缺目标库时整批拒绝且不落库（原子性）",
		codeOf(r) == 5555 && batchCount("SELECT COUNT(*) FROM core_sql_orders WHERE `text` = ?", atomText) == 0,
		fmt.Sprintf("code=%v text=%s", r["code"], textOf(r)))

	r = batchSubmit(devTok, uniq("空批次"), 1, 0, "none", []map[string]any{})
	check("T7.11", "空明细提交被拒", codeOf(r) == 5555, fmt.Sprintf("code=%v text=%s", r["code"], textOf(r)))

	// 还原本组改动的目标行（工单与执行记录保留，仅把 users 数据复原）
	_, _ = target.Exec("UPDATE "+*tableName+" SET email = ? WHERE "+*idColumn+" = 1", origB1)
	_, _ = target.Exec("UPDATE "+*tableName+" SET email = ? WHERE "+*idColumn+" = 3", origB3)

	fmt.Println("\n=== 组 8 列表按项目聚合（我的工单 / 记录） ===")
	mineSeen := listBatchPages(devTok, "/api/v2/common/list", 3, 6)
	check("T8.1", "我的工单：带 batch_id 且同批次子工单不跨页",
		len(mineSeen) > 0 && splitBatches(mineSeen) == 0,
		fmt.Sprintf("batches=%d split=%d", len(mineSeen), splitBatches(mineSeen)))

	recSeen := listBatchPages(adminTok, "/api/v2/record/list", 3, 6)
	check("T8.2", "记录列表：带 batch_id 且同批次子工单不跨页",
		len(recSeen) > 0 && splitBatches(recSeen) == 0,
		fmt.Sprintf("batches=%d split=%d", len(recSeen), splitBatches(recSeen)))

	// 编号规则不变量：普通工单 = 自增 id；项目子工单 = 项目号-序号（项目号 = 项目内首条子工单 id）
	badNo := batchCount(`SELECT COUNT(*) FROM core_sql_orders o WHERE
		(o.batch_id = '' AND o.work_id <> CAST(o.id AS CHAR))
		OR (o.batch_id <> '' AND (
			o.batch_id <> CAST((SELECT MIN(c.id) FROM core_sql_orders c WHERE c.batch_id = o.batch_id) AS CHAR)
			OR o.work_id <> CONCAT(o.batch_id, '-', (SELECT COUNT(*) FROM core_sql_orders c WHERE c.batch_id = o.batch_id AND c.id <= o.id))
		))`)
	check("T8.3", "SQL 工单编号全部符合自增 id 规则（含历史数据）", badNo == 0,
		fmt.Sprintf("不符合=%d", badNo))

	badQ := batchCount("SELECT COUNT(*) FROM core_query_orders WHERE work_id <> CAST(id AS CHAR)")
	check("T8.4", "查询工单编号全部符合自增 id 规则（含历史数据）", badQ == 0,
		fmt.Sprintf("不符合=%d", badQ))

	// 列表里同一项目的子工单必须严格按 -1、-2、-3… 升序。
	// 造一个「状态混合」的批次（审一条后：1 条待执行 + 2 条审核中），
	// 列表按「待审批优先」排序时最容易把 -1 挤到后面，正是这个用例要覆盖的场景。
	sortText := uniq("排序校验")
	sr := batchSubmit(devTok, sortText, 1, 0, "none", []map[string]any{
		batchItem(noop), batchItem(noop), batchItem(noop),
	})
	sortBid, _ := sr["payload"].(string)
	sw := batchWorkIds(sortBid)
	if len(sw) > 0 {
		call(dbaTok, "/api/v2/audit/order/state", map[string]any{"tp": "agree", "work_id": sw[0], "flag": *approveFlag})
	}
	gotSeq, seqOK, prev := []int{}, true, 0
	for _, row := range auditRows(adminTok, 1, 50) {
		if bid, _ := row["batch_id"].(string); bid != sortBid {
			continue
		}
		n := suffixNum(row["work_id"].(string))
		gotSeq = append(gotSeq, n)
		if n != prev+1 {
			seqOK = false
		}
		prev = n
	}
	check("T8.5", "状态混合的项目里子工单仍严格按 -1、-2、-3… 排列",
		len(gotSeq) == 3 && seqOK, fmt.Sprintf("batch=%s 序号=%v", sortBid, gotSeq))

	fmt.Printf("\n=== 汇总：用例 %d，PASS %d，SKIP %d，FAIL %d ===\n", pass+skipped+len(fails), pass, skipped, len(fails))
	fmt.Printf("测试数据保留（工单文本前缀 %q），可在界面上核对；配置类改动已还原。\n", marker)
	for _, f := range fails {
		fmt.Println("  FAIL", f)
	}
	if len(fails) > 0 {
		os.Exit(1)
	}
}
