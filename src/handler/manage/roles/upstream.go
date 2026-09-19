package roles

import (
	"net/http"
	"regexp"
	"time"

	"Yearning-go/src/handler/common"

	"github.com/cookieY/yee"
)

// bytebaseRef 为当前内化的 Bytebase 版本，需与 engine/internal/bytebase/UPSTREAM
// 的 ref 保持一致（升级后手工改这一行）。
const bytebaseRef = "3.22.1"

// bytebaseLatestURL 会 302 到 .../releases/tag/<tag>，无需 GitHub API（避免限流）。
const bytebaseLatestURL = "https://github.com/bytebase/bytebase/releases/latest"

var reReleaseTag = regexp.MustCompile(`/releases/tag/([^/]+)$`)

type upstreamStatus struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	HasUpdate bool   `json:"hasUpdate"`
}

// SuperUpstreamCheck 检测 Bytebase 是否有更新的 release。
//
// 这里只做检测：上游规则是随引擎一起编译的 Go 代码，运行期无法替换，
// 更新必须重新同步源码并重新编译、发布（页面不会自动改任何文件）。
func SuperUpstreamCheck(c yee.Context) (err error) {
	// ponytail: 固定 20s 上限；实测本机到 GitHub 偶发 SSL 握手超时，10s 会误报不可用。
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(bytebaseLatestURL)
	if err != nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("无法访问上游仓库: "+err.Error()))
	}
	defer resp.Body.Close()

	m := reReleaseTag.FindStringSubmatch(resp.Request.URL.Path)
	if m == nil {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE("无法解析上游最新版本"))
	}

	latest := m[1]
	return c.JSON(http.StatusOK, common.SuccessPayload(upstreamStatus{
		Current:   bytebaseRef,
		Latest:    latest,
		HasUpdate: latest != bytebaseRef,
	}))
}
