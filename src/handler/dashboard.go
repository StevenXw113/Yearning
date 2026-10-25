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
}

func DashBanner(c yee.Context) (err error) {
	var b bannerCount
	user := new(factory.Token).JwtParse(c)
	model.DB().Model(model.CoreAccount{}).Count(&b.User)
	model.DB().Model(model.CoreDataSource{}).Count(&b.Source)
	model.DB().Model(model.CoreTotalTickets{}).Order("date desc ").Limit(7).Find(&b.TotalOrder)
	// 同表计数合并为条件聚合:原来工单表 4 次全扫、查询表 2 次全扫
	var sqlRow struct {
		Total     int64
		SelfDDL   int64
		SelfDML   int64
		SelfAudit int64
	}
	model.DB().Model(model.CoreSqlOrder{}).Select(
		"COUNT(*) AS total",
		"IFNULL(SUM(`type` = 0 AND username = ?), 0) AS self_ddl",
		"IFNULL(SUM(`type` = 1 AND username = ?), 0) AS self_dml",
		"IFNULL(SUM(`status` = 2 AND assigned LIKE ?), 0) AS self_audit",
		user.Username, user.Username, "%"+user.Username+"%",
	).Scan(&sqlRow)
	b.Order, b.SelfDDL, b.SelfDML, b.SelfAudit = sqlRow.Total, sqlRow.SelfDDL, sqlRow.SelfDML, sqlRow.SelfAudit
	var queryRow struct {
		Total     int64
		SelfQuery int64
	}
	model.DB().Model(model.CoreQueryOrder{}).Select(
		"COUNT(*) AS total",
		"IFNULL(SUM(username = ?), 0) AS self_query",
		user.Username,
	).Scan(&queryRow)
	b.Query, b.SelfQuery = queryRow.Total, queryRow.SelfQuery
	return c.JSON(http.StatusOK, common.SuccessPayload(b))
}

func DashTop(c yee.Context) (err error) {
	var source []groupBy
	model.DB().Model(model.CoreSqlOrder{}).Select("source, count(*) as c").Group("source").Order("c desc").Limit(10).Scan(&source)
	return c.JSON(http.StatusOK, common.SuccessPayload(source))
}
