package audit

import (
	"context"
	"fmt"
	"time"

	"Yearning-go/src/engine"
	"Yearning-go/src/lib/calls"
	"Yearning-go/src/model"

	"engine/api"
)

// recheckRedisOrder 执行前复核 Redis 工单。
//
// 审批到执行之间规则可能被收紧（也可能工单是绕过检测直接提交的），执行入口
// 必须再跑一遍审核。复核要带上运行时事实——部署形态、生产标记、实例版本、
// 键状态快照——少给一项，对应的拓扑/键状态规则就静默失效，复核等于没跑。
//
// 非 Redis 数据源直接放行：SQL 由引擎执行链路自己校验。
func recheckRedisOrder(order *model.CoreSqlOrder, source *model.CoreDataSource, password string, rule *engine.AuditRole) error {
	if order == nil || source == nil || source.DBType != model.DBTypeRedis {
		return nil
	}
	client, err := calls.Client()
	if err != nil {
		return fmt.Errorf("引擎不可用，未做执行前复核: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	src := &api.DataSource{
		Ip:       source.IP,
		Port:     int32(source.Port),
		Username: source.Username,
		Password: password,
		Ca:       source.CAFile,
		Cert:     source.Cert,
		Key:      source.KeyFile,
		Kind:     calls.DataSourceKind(source.DBType),
		AuthDb:   source.AuthDatabase,
	}
	keySnapshot := calls.RedisAudit(src, source, order.SQL, order.DataBase)
	rep, err := client.Check(ctx, &api.CheckRequest{
		Sql:         order.SQL,
		Schema:      order.DataBase,
		KeySnapshot: keySnapshot,
		Source:      src,
		Lang:        model.C.General.Lang,
		Rule:        engine.AuditRoleToAPI(rule),
		Mode:        engine.CheckModeWrite,
	})
	if err != nil || rep == nil || !rep.Ok {
		return fmt.Errorf("执行前复核失败: %v", calls.CombineReplyErr(rep, err))
	}
	for _, r := range rep.Records {
		// 0=通过、1=error 阻断，2=warning / 3=observe 放行
		if r.Level == 1 {
			return fmt.Errorf("执行前复核发现阻断级问题: %s", r.Error)
		}
	}
	return nil
}
