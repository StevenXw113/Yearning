// Copyright 2019 HenryYee.
//
// Licensed under the AGPL, Version 3.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.gnu.org/licenses/agpl-3.0.en.html
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package pusher

import (
	"Yearning-go/src/model"
	"crypto/tls"
	"fmt"
	"github.com/cookieY/yee/logger"
	"github.com/wneessen/go-mail"
	"strings"
)

var TemoplateTestMail = `
<html>
<body>
	<div style="text-align:center;">
		<h1>Yearning 3.0</h1>
		<h2>此邮件是测试邮件！</h2>
	</div>
</body>
</html>
`

var TmplMail = `
<html>
<body>
<h1>Yearning 工单%s通知</h1>
<br><p>工单号: %s</p>
<br><p>发起人: %s</p>
<br><p>地址: <a href="%s">%s</a></p>
<br><p>状态: %s</p>
</body>
</html>
`

var Tmpl2Mail = `
<html>
<body>
<h1>Yearning 工单%s通知</h1>
<br><p>工单号: %s</p>
<br><p>发起人: %s</p>
<br><p>下一步操作人: %s <p>
<br><p>地址: <a href="%s">%s</a></p>
<br><p>状态: %s</p>
</body>
</html>
`

func NewMessagePusher(orderId string) *Msg {
	return &Msg{orderId: orderId}
}

// SystemAlert 基础设施级告警（引擎不可用/自愈动作等），与工单无关、没有明确的
// 单个收件人，走钉钉群 WebHook（配置了才发）+ 日志。不随工单邮件骚扰个人。
// 防风暴由调用方负责（健康翻转只各告警一次）。
func SystemAlert(subject, content string) {
	msg := *model.GloMessage.Load()
	if msg.Ding && msg.WebHook != "" {
		go PusherMessages(msg, fmt.Sprintf("【%s】\n%s", subject, content))
	}
	logger.DefaultLogger.Errorf("系统告警: %s: %s", subject, content)
}

func (m *Msg) Order() *Msg {
	var user model.CoreAccount
	var order model.CoreSqlOrder
	model.DB().Select("work_id,username,text,assigned,source").Where("work_id =?", m.orderId).First(&order)
	model.DB().Select("email").Where("username = ?", order.Username).First(&user)
	m.ll.ToUser = []model.CoreAccount{user}
	m.ll.Message = *model.GloMessage.Load()
	m.orderInfo = order
	return m
}

func (m *Msg) Query() *Msg {
	var user model.CoreAccount
	var order model.CoreQueryOrder
	model.DB().Select("work_id,username,text,assigned").Where("work_id =?", m.orderId).First(&order)
	model.DB().Select("email").Where("username = ?", order.Username).First(&user)
	m.ll.ToUser = []model.CoreAccount{user}
	m.ll.Message = *model.GloMessage.Load()
	m.queryInfo = order
	return m
}

func (m *Msg) QueryBuild(status StatusType) *OrderTPL {
	tpl := new(OrderTPL)
	tpl.ll = m.ll
	switch status {
	case RejectStatus:
		tpl.pushTpl = dingMsgTplHandler("已驳回", m.queryInfo)
		tpl.mailTpl = fmt.Sprintf(TmplMail, "查询申请", m.queryInfo.WorkId, m.queryInfo.Username, model.GloOther.Load().Domain, model.GloOther.Load().Domain, "已驳回")
	case AgreeStatus:
		tpl.pushTpl = dingMsgTplHandler("已同意", m.queryInfo)
		tpl.mailTpl = fmt.Sprintf(TmplMail, "查询申请", m.queryInfo.WorkId, m.queryInfo.Username, model.GloOther.Load().Domain, model.GloOther.Load().Domain, "已同意")
	case SummitStatus:
		model.DB().Select("email").Where("username IN (?)", strings.Split(m.queryInfo.Assigned, ",")).Find(&m.ll.ToUser)
		tpl.pushTpl = dingMsgTplHandler("已提交", m.queryInfo)
		tpl.mailTpl = fmt.Sprintf(TmplMail, "查询申请", m.queryInfo.WorkId, m.queryInfo.Username, model.GloOther.Load().Domain, model.GloOther.Load().Domain, "已提交")
	default:
		model.DefaultLogger.Error("unknown status")
	}
	return tpl
}

func (m *Msg) OrderBuild(status StatusType) *OrderTPL {
	tpl := new(OrderTPL)
	tpl.ll = m.ll
	switch status {
	case ExecuteStatus:
		tpl.pushTpl = dingMsgTplHandler("已执行", m.orderInfo)
		tpl.mailTpl = fmt.Sprintf(TmplMail, "执行", m.orderInfo.WorkId, m.orderInfo.Username, model.GloOther.Load().Domain, model.GloOther.Load().Domain, "执行成功")
	case RejectStatus:
		tpl.pushTpl = dingMsgTplHandler("已驳回", m.orderInfo)
		tpl.mailTpl = fmt.Sprintf(TmplMail, "查询申请", m.orderInfo.WorkId, m.orderInfo.Username, model.GloOther.Load().Domain, model.GloOther.Load().Domain, "已驳回")
	case SummitStatus:
		model.DB().Select("email").Where("username IN (?)", strings.Split(m.orderInfo.Assigned, ",")).Find(&m.ll.ToUser)
		tpl.pushTpl = dingMsgTplHandler("已提交", m.orderInfo)
		tpl.mailTpl = fmt.Sprintf(TmplMail, "提交", m.orderInfo.WorkId, m.orderInfo.Username, model.GloOther.Load().Domain, model.GloOther.Load().Domain, "已提交")
	case FailedStatus:
		tpl.pushTpl = dingMsgTplHandler("执行失败", m.orderInfo)
		tpl.mailTpl = fmt.Sprintf(TmplMail, "执行", m.orderInfo.WorkId, m.orderInfo.Username, model.GloOther.Load().Domain, model.GloOther.Load().Domain, "执行失败")
	case NextStepStatus:
		model.DB().Select("email").Where("username IN (?)", strings.Split(m.orderInfo.Assigned, ",")).Find(&m.ll.ToUser)
		tpl.pushTpl = dingMsgTplHandler("已转交至下一操作人", m.orderInfo)
		tpl.mailTpl = fmt.Sprintf(Tmpl2Mail, "转交", m.orderInfo.WorkId, m.orderInfo.Username, m.orderInfo.Assigned, model.GloOther.Load().Domain, model.GloOther.Load().Domain, "已转交至下一操作人")
	case UndoStatus:
		tpl.pushTpl = dingMsgTplHandler("已撤销", m.orderInfo)
		tpl.mailTpl = fmt.Sprintf(TmplMail, "提交", m.orderInfo.WorkId, m.orderInfo.Username, model.GloOther.Load().Domain, model.GloOther.Load().Domain, "已撤销")
	default:
		model.DefaultLogger.Error("unknown status")
	}
	return tpl
}

func (tpl *OrderTPL) Push() {
	message := model.GloMessage.Load()
	if message.Mail {
		for _, i := range tpl.ll.ToUser {
			if i.Email != "" {
				go SendMail(i.Email, tpl.ll.Message, tpl.mailTpl)
			}
		}
	}
	if message.Ding {
		if message.WebHook != "" {
			go PusherMessages(tpl.ll.Message, tpl.pushTpl)
		}
	}
}

// SendMail 经 SMTP 推送告警邮件(go-mail,替代 2016 年起停更的 gomail.v2)。
func SendMail(addr string, cfg model.Message, tmpl string) {
	m := mail.NewMsg()
	if err := m.From(cfg.User); err != nil {
		logger.DefaultLogger.Errorf("send mail: %s", err.Error())
		return
	}
	if err := m.To(addr); err != nil {
		logger.DefaultLogger.Errorf("send mail: %s", err.Error())
		return
	}
	m.Subject("Yearning消息推送!")
	m.SetBodyString(mail.TypeTextHTML, tmpl)
	opts := []mail.Option{
		mail.WithPort(cfg.Port),
		mail.WithSMTPAuth(mail.SMTPAuthPlain),
		mail.WithUsername(cfg.User),
		mail.WithPassword(cfg.Password),
		// gomail 语义:非 SSL 走机会性 STARTTLS(服务器支持才升级,不支持则明文投递)
		mail.WithTLSPolicy(mail.TLSOpportunistic),
	}
	if cfg.Ssl {
		// 隐式 TLS;校验服务器证书与主机名:跳过校验会让 SMTP 账号密码与邮件正文暴露给中间人
		opts = append(opts, mail.WithSSLPort(false), mail.WithTLSConfig(&tls.Config{ServerName: cfg.Host}))
	}
	c, err := mail.NewClient(cfg.Host, opts...)
	if err != nil {
		logger.DefaultLogger.Errorf("send mail: %s", err.Error())
		return
	}
	if err := c.DialAndSend(m); err != nil {
		logger.DefaultLogger.Errorf("send mail:%s", err.Error())
		return
	}
}
