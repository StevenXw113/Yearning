// Command checkdiff 用同一批真实工单 SQL 与同一份审核规则，分别在「线上正在跑的引擎」与
// 「候选引擎」上各跑一遍 Check，逐条比对审核结果。
//
// 没有预发环境时，它替代“先上预发看看”这一步：只要两边结果一致（或差异已人工确认），
// 再替换线上容器；差异就是升级引入的审核行为变化，必须逐条过一遍。
//
// 用法（在仓库根目录执行）：
//
//	go run ./tools/checkdiff \
//	  -old 10.0.0.5:13307 -new 127.0.0.1:13307 \
//	  -meta-dsn 'user:pwd@tcp(10.0.0.30:3306)/Yearning_go' -limit 200
//
// 退出码：0 = 无差异；1 = 有差异；2 = 运行失败（配置/连接问题）。
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"Yearning-go/src/engine"

	"engine/api"

	_ "github.com/go-sql-driver/mysql"
)

type statement struct {
	workID string
	schema string
	sql    string
}

// maxLines 单笔工单最多打印多少行差异明细。
const maxLines = 12

func main() {
	oldAddr := flag.String("old", "", "线上正在跑的引擎地址，host:port（必填）")
	newAddr := flag.String("new", "", "候选引擎地址，host:port（必填）")
	metaDSN := flag.String("meta-dsn", "", "元数据库 DSN：从中取真实工单 SQL 与全局审核规则")
	sqlFile := flag.String("sql-file", "", "可选：改为从文件读 SQL（每行一个工单，单行内不能有换行）")
	ruleFile := flag.String("rule-file", "", "可选：用文件中的 audit_role JSON 作为审核规则（默认取元数据库全局规则）")
	limit := flag.Int("limit", 200, "最多比对多少笔工单")
	show := flag.Int("show", 8, "最多打印多少条差异明细")
	timeout := flag.Duration("timeout", 60*time.Second, "单笔 Check 的超时")
	// token 引擎共享访问令牌：引擎启用认证（-auth-token）后对拍必须带同一令牌，
	// 否则所有 Check 都被拒 Unauthenticated，发布门禁失效
	token := flag.String("token", os.Getenv("ENGINE_AUTH_TOKEN"), "引擎共享访问令牌（默认取环境变量 ENGINE_AUTH_TOKEN；引擎未启用认证时留空）")
	flag.Parse()

	if *oldAddr == "" || *newAddr == "" {
		flag.Usage()
		os.Exit(2)
	}

	rule, err := loadRule(*metaDSN, *ruleFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取审核规则失败:", err)
		os.Exit(2)
	}
	corpus, err := loadCorpus(*metaDSN, *sqlFile, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, "读取工单 SQL 失败:", err)
		os.Exit(2)
	}
	if len(corpus) == 0 {
		fmt.Fprintln(os.Stderr, "没有可比对的 SQL")
		os.Exit(2)
	}

	oldCaller, err := dial(*oldAddr, *token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接线上引擎 %s 失败: %v\n", *oldAddr, err)
		os.Exit(2)
	}
	newCaller, err := dial(*newAddr, *token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接候选引擎 %s 失败: %v\n", *newAddr, err)
		os.Exit(2)
	}

	fmt.Printf("线上 %s  vs  候选 %s\n", *oldAddr, *newAddr)
	fmt.Printf("比对 %d 笔工单（规则取 %s）\n\n", len(corpus), ruleSource(*ruleFile))

	diffs := 0
	for _, st := range corpus {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		req := &api.CheckRequest{Sql: st.sql, Schema: st.schema, Rule: rule, Lang: "zh_CN"}
		o, oerr := oldCaller.Check(ctx, req)
		n, nerr := newCaller.Check(ctx, req)
		cancel()

		lines := compare(o, oerr, n, nerr)
		if len(lines) == 0 {
			continue
		}
		diffs++
		if diffs <= *show {
			fmt.Printf("[%d] 工单 %s（%s）\n    SQL : %s\n", diffs, short(st.workID), st.schema, oneLine(st.sql))
			// 一笔工单可能有多条语句命中差异，明细限长，避免刷屏
			for i, l := range lines {
				if i >= maxLines {
					fmt.Printf("    …还有 %d 行差异明细\n", len(lines)-i)
					break
				}
				fmt.Println("   ", l)
			}
		}
	}

	fmt.Println()
	if diffs == 0 {
		fmt.Printf("结论：%d 笔全部一致，可以发布。\n", len(corpus))
		return
	}
	fmt.Printf("结论：%d/%d 笔存在差异", diffs, len(corpus))
	if diffs > *show {
		fmt.Printf("（仅展示前 %d 条）", *show)
	}
	fmt.Printf("。逐条确认是「预期收紧/放松」还是「回归」，确认后再发布。\n")
	os.Exit(1)
}

// compare 返回两边结果的差异描述（空表示一致）。
func compare(o *api.CheckReply, oerr error, n *api.CheckReply, nerr error) []string {
	if (oerr == nil) != (nerr == nil) {
		return []string{fmt.Sprintf("线上 err=%v / 候选 err=%v", oerr, nerr)}
	}
	if oerr != nil {
		return nil // 两边都连不上或都报错，不算行为差异
	}
	if o.Ok != n.Ok {
		return []string{fmt.Sprintf("整体结果不同：线上 ok=%v / 候选 ok=%v", o.Ok, n.Ok)}
	}
	var out []string
	or, nr := o.Records, n.Records
	if len(or) != len(nr) {
		out = append(out, fmt.Sprintf("记录条数不同：线上 %d / 候选 %d", len(or), len(nr)))
	}
	for i := 0; i < len(or) && i < len(nr); i++ {
		a, b := or[i], nr[i]
		if a.Level == b.Level && a.Status == b.Status && a.Error == b.Error {
			continue
		}
		out = append(out,
			fmt.Sprintf("线上 level=%d status=%s %s", a.Level, a.Status, oneLine(a.Error)),
			fmt.Sprintf("候选 level=%d status=%s %s", b.Level, b.Status, oneLine(b.Error)),
		)
	}
	return out
}

// loadRule 读取审核规则：优先用文件，其次取元数据库里的全局规则。
func loadRule(metaDSN, ruleFile string) (*api.AuditRole, error) {
	var raw []byte
	if ruleFile != "" {
		b, err := os.ReadFile(ruleFile)
		if err != nil {
			return nil, err
		}
		raw = b
	} else {
		db, err := sql.Open("mysql", metaDSN)
		if err != nil {
			return nil, err
		}
		defer db.Close()
		if err := db.QueryRow("SELECT audit_role FROM core_global_configurations LIMIT 1").Scan(&raw); err != nil {
			return nil, fmt.Errorf("读全局规则失败（可用 -rule-file 指定）: %w", err)
		}
	}
	var role engine.AuditRole
	if err := json.Unmarshal(raw, &role); err != nil {
		return nil, err
	}
	return engine.AuditRoleToAPI(&role), nil
}

// loadCorpus 读取待比对语句：优先用文件（每行一个工单），否则取元数据库最近 N 笔工单。
func loadCorpus(metaDSN, sqlFile string, limit int) ([]statement, error) {
	if sqlFile != "" {
		b, err := os.ReadFile(sqlFile)
		if err != nil {
			return nil, err
		}
		var out []statement
		for i, line := range strings.Split(string(b), "\n") {
			if s := strings.TrimSpace(line); s != "" {
				out = append(out, statement{workID: fmt.Sprintf("line-%d", i+1), sql: s})
			}
		}
		return out, nil
	}
	db, err := sql.Open("mysql", metaDSN)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.Query(
		"SELECT work_id, data_base, `sql` FROM core_sql_orders WHERE `sql` <> '' ORDER BY id DESC LIMIT ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []statement
	for rows.Next() {
		var st statement
		if err := rows.Scan(&st.workID, &st.schema, &st.sql); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// engineCaller 引擎 HTTP 客户端(对拍工具只需 Check,不引入主服务的单例/配置)。
type engineCaller struct {
	base  string
	token string
	http  *http.Client
}

// dial 连接引擎并做一次探活 Check(与旧 gRPC 版语义一致:连不上即退出码 2)。
func dial(addr, token string) (*engineCaller, error) {
	base := strings.TrimSuffix(strings.TrimSpace(addr), "/")
	if !strings.Contains(base, "://") {
		base = "http://" + base
	}
	c := &engineCaller{base: base, token: token, http: &http.Client{}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := c.Check(ctx, &api.CheckRequest{Sql: "SELECT 1"}); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *engineCaller) Check(ctx context.Context, req *api.CheckRequest) (*api.CheckReply, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/check", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var rep api.CheckReply
	if err := json.NewDecoder(resp.Body).Decode(&rep); err != nil {
		return nil, err
	}
	return &rep, nil
}

func ruleSource(ruleFile string) string {
	if ruleFile != "" {
		return ruleFile
	}
	return "元数据库全局规则"
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(s, "\n", " ")), " ")
	if len(s) > 160 {
		return s[:160] + "…"
	}
	return s
}
