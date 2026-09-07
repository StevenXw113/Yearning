package pusher

import (
	"Yearning-go/src/i18n"
	"Yearning-go/src/model"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

type imCryGeneric struct {
	Assigned string
	WorkId   string
	Source   string
	Username string
	Text     string
}

var Commontext = `
{
        "msgtype": "markdown",
        "markdown": {
                "title": "Yearning",
                "text": "## Yearning工单通知 \n\n **工单编号:** $WORKID\n\n **数据源:** $SOURCE\n\n **工单说明:** $TEXT\n\n **提交人员:** <font color = \"#78beea\">$USER</font> \n \n **下一步操作人:** <font color=\"#fe8696\">$AUDITOR</font> \n \n **平台地址:** [$HOST]($HOST) \n \n  **状态:** <font color=\"#1abefa\">$STATE</font> \n \n"
        }
}

`

func PusherMessages(msg model.Message, sv string) {
	//请求地址模板

	//创建一个请求

	hook := msg.WebHook

	if msg.Key != "" {
		hook = Sign(msg.Key, msg.WebHook)
	}
	req, err := http.NewRequest("POST", hook, strings.NewReader(sv))
	if err != nil {
		model.DefaultLogger.Errorf("ding request error: %v", err)
		return
	}

	// 不关闭证书校验：WebHook 签名与工单内容都会经过该连接
	client := &http.Client{Timeout: 10 * time.Second}
	//设置请求头
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	//发送请求
	resp, err := client.Do(req)

	if err != nil {
		model.DefaultLogger.Errorf("ding push error: %v", err)
		return
	}
	defer resp.Body.Close()
	// 响应体不再写入日志：含工单号、提交人与 SQL 说明
	_, _ = io.Copy(io.Discard, resp.Body)
}

func Sign(secret, hook string) string {
	timestamp := time.Now().UnixNano() / 1e6
	stringToSign := fmt.Sprintf("%d\n%s", timestamp, secret)
	sign := hmacSha256(stringToSign, secret)
	// sign 是标准 base64，可能包含 + / =，必须转义后再拼进查询串
	return fmt.Sprintf("%s&timestamp=%d&sign=%s", hook, timestamp, neturl.QueryEscape(sign))
}

func dingMsgTplHandler(state string, generic interface{}) string {

	var order imCryGeneric
	switch v := generic.(type) {
	case model.CoreSqlOrder:
		order = imCryGeneric{
			Assigned: v.Assigned,
			WorkId:   v.WorkId,
			Source:   v.Source,
			Username: v.Username,
			Text:     v.Text,
		}
	case model.CoreQueryOrder:
		order = imCryGeneric{
			Assigned: v.Assigned,
			WorkId:   v.WorkId + i18n.DefaultLang.Load(i18n.INFO_QUERY),
			Source:   i18n.DefaultLang.Load(i18n.ER_QUERY_NO_DATA_SOURCE),
			Username: v.Username,
			Text:     v.Text,
		}
	}
	text := Commontext
	if !stateHandler(state) {
		order.Assigned = "无"
	}
	text = strings.Replace(text, "$STATE", state, -1)
	text = strings.Replace(text, "$WORKID", order.WorkId, -1)
	text = strings.Replace(text, "$SOURCE", order.Source, -1)
	text = strings.Replace(text, "$HOST", model.GloOther.Load().Domain, -1)
	text = strings.Replace(text, "$USER", order.Username, -1)
	text = strings.Replace(text, "$AUDITOR", order.Assigned, -1)
	text = strings.Replace(text, "$TEXT", order.Text, -1)
	return text
}

func stateHandler(state string) bool {
	switch state {
	case i18n.DefaultLang.Load(i18n.INFO_TRANSFERRED_TO_NEXT_AGENT), i18n.DefaultLang.Load(i18n.INFO_SUBMITTED):
		return true
	}
	return false
}

func hmacSha256(stringToSign string, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}
