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

package handler

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/lib/factory"
	"Yearning-go/src/model"
	"github.com/cookieY/yee"
	"net/http"
	"time"
)

type groupBy struct {
	Source string `json:"source"`
	C      int    `json:"count"`
	Time   string `json:"time"`
	Type   string `json:"type"`
}

type bannerCount struct {
	User       int64                    `json:"user"`
	TotalOrder []model.CoreTotalTickets `json:"total_order"`
	Order      int64                    `json:"order"`
	Query      int64                    `json:"query"`
	Source     int64                    `json:"source"`
	SelfDDL    int64                    `json:"self_ddl"`
	SelfDML    int64                    `json:"self_dml"`
	SelfAudit  int64                    `json:"self_audit"`
	SelfQuery  int64                    `json:"self_query"`
	WindowDays int                      `json:"window_days"`
}

// bannerWindowDays：「我的」三项只看近期数据。不加窗口时条件聚合要扫整表，
// 表上带 SQL/Text/OSCInfo 三个 longtext，工单量大后首页每次刷新都是聚类全扫。
const bannerWindowDays = 90

func DashBanner(c yee.Context) (err error) {
	var b bannerCount
	user := new(factory.Token).JwtParse(c)
	model.DB().Model(model.CoreAccount{}).Count(&b.User)
	model.DB().Model(model.CoreDataSource{}).Count(&b.Source)
	model.DB().Model(model.CoreTotalTickets{}).Order("date desc ").Limit(7).Find(&b.TotalOrder)
	// 总数保持全量：COUNT 走二级索引，不读 longtext，成本可接受
	model.DB().Model(model.CoreSqlOrder{}).Count(&b.Order)
	model.DB().Model(model.CoreQueryOrder{}).Count(&b.Query)

	// 「我的」限时间窗口，date 是 varchar(50) 的 "2006-01-02 15:04"，前缀比较可走 date_idx
	b.WindowDays = bannerWindowDays
	since := time.Now().AddDate(0, 0, -bannerWindowDays).Format("2006-01-02")

	var sqlRow struct {
		SelfDDL   int64
		SelfDML   int64
		SelfAudit int64
	}
	model.DB().Model(model.CoreSqlOrder{}).
		Where("date >= ?", since).
		Select("IFNULL(SUM(`type` = 0 AND username = ?), 0) AS self_ddl,"+
			"IFNULL(SUM(`type` = 1 AND username = ?), 0) AS self_dml,"+
			"IFNULL(SUM(`status` = 2 AND assigned LIKE ?), 0) AS self_audit",
			user.Username, user.Username, "%"+user.Username+"%").
		Scan(&sqlRow)
	b.SelfDDL, b.SelfDML, b.SelfAudit = sqlRow.SelfDDL, sqlRow.SelfDML, sqlRow.SelfAudit

	var queryRow struct {
		SelfQuery int64
	}
	model.DB().Model(model.CoreQueryOrder{}).
		Where("date >= ?", since).
		Select("IFNULL(SUM(username = ?), 0) AS self_query", user.Username).
		Scan(&queryRow)
	b.SelfQuery = queryRow.SelfQuery
	return c.JSON(http.StatusOK, common.SuccessPayload(b))
}

func DashTop(c yee.Context) (err error) {
	var source []groupBy
	model.DB().Model(model.CoreSqlOrder{}).Select("source, count(*) as c").Group("source").Order("c desc").Limit(10).Scan(&source)
	return c.JSON(http.StatusOK, common.SuccessPayload(source))
}
