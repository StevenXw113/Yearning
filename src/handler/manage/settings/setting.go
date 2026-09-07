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

package settings

import (
	"Yearning-go/src/handler/common"
	"Yearning-go/src/i18n"
	"Yearning-go/src/lib/pusher"
	"Yearning-go/src/model"
	"encoding/json"
	"github.com/cookieY/yee"
	"net/http"
)

type set struct {
	Message model.Message `json:"message"`
	Other   model.Other   `json:"other"`
	AI      model.AI      `json:"ai"`
}

type delOrder struct {
	Date []string `json:"date"`
	Tp   bool     `json:"tp"`
}

// maskSensitive 清空设置里的凭据字段，避免明文口令出现在响应体中
func maskSensitive(u *set) {
	u.Message.Password = ""
	u.Message.Key = ""
	u.AI.APIKey = ""
}

func SuperFetchSetting(c yee.Context) (err error) {

	var k model.CoreGlobalConfiguration

	model.DB().Select("message,other,ai").First(&k)

	s := set{Message: model.Message{}, Other: model.Other{}, AI: model.AI{}}
	_ = k.Message.UnmarshalToJSON(&s.Message)
	_ = k.Other.UnmarshalToJSON(&s.Other)
	_ = k.AI.UnmarshalToJSON(&s.AI)
	maskSensitive(&s)

	return c.JSON(http.StatusOK, common.SuccessPayload(s))
}

func SuperSaveSetting(c yee.Context) (err error) {

	u := new(set)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err)
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}
	other, _ := json.Marshal(u.Other)
	message, _ := json.Marshal(u.Message)
	ai, _ := json.Marshal(u.AI)

	if !u.Other.Query {
		model.DB().Model(model.CoreQueryOrder{}).Where("`status` in (?)", []int{1, 2}).Updates(&model.CoreQueryOrder{Status: 3})
	}

	model.DB().Model(model.CoreGlobalConfiguration{}).Where("1=1").Updates(&model.CoreGlobalConfiguration{Other: other, Message: message, AI: ai})
	model.GloOther.Store(&u.Other)
	model.GloMessage.Store(&u.Message)
	model.GloAI.Store(&u.AI)
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_DATA_IS_EDIT)))
}

func SuperTestSetting(c yee.Context) (err error) {

	el := c.QueryParam("test")
	u := new(set)
	if err = c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}

	switch el {
	case "mail":
		go pusher.SendMail(u.Message.ToUser, u.Message, pusher.TemoplateTestMail)
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.MAIL_TEST)))
	case "ding":
		go pusher.PusherMessages(u.Message, pusher.Commontext)
		return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.WEBHOOK_TEST)))
	}
	return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
}

func SuperDelOrder(c yee.Context) (err error) {
	u := new(delOrder)
	if err := c.Bind(u); err != nil {
		c.Logger().Error(err.Error())
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_BIND)))
	}

	if len(u.Date) != 2 {
		return c.JSON(http.StatusOK, common.ERR_COMMON_TEXT_MESSAGE(i18n.DefaultLang.Load(i18n.ER_REQ_FAKE)))
	}
	// 删除必须整体处于同一事务中：任一环节失败都要回滚，避免留下孤儿记录
	if u.Tp {
		go func() {
			var order []model.CoreQueryOrder
			tx := model.DB().Begin()
			if err := tx.Select("work_id").Where("`date` >= ? and `date` <= ?", u.Date[0], u.Date[1]).Find(&order).Error; err != nil {
				tx.Rollback()
				return
			}
			if err := tx.Where("`date` >= ? and `date` <= ?", u.Date[0], u.Date[1]).Delete(&model.CoreQueryOrder{}).Error; err != nil {
				tx.Rollback()
				return
			}
			for _, i := range order {
				if err := tx.Where("work_id =?", i.WorkId).Delete(&model.CoreQueryRecord{}).Error; err != nil {
					tx.Rollback()
					return
				}
			}
			tx.Commit()
		}()
	} else {
		go func() {
			var order []model.CoreSqlOrder
			tx := model.DB().Begin()
			if err := tx.Select("work_id").Where("`date` >= ? and `date` <= ?", u.Date[0], u.Date[1]).Find(&order).Error; err != nil {
				tx.Rollback()
				return
			}
			for _, i := range order {
				if err := tx.Where("work_id =?", i.WorkId).Delete(&model.CoreSqlOrder{}).Error; err != nil {
					tx.Rollback()
					return
				}
				if err := tx.Where("work_id =?", i.WorkId).Delete(&model.CoreRollback{}).Error; err != nil {
					tx.Rollback()
					return
				}
				if err := tx.Where("work_id =?", i.WorkId).Delete(&model.CoreSqlRecord{}).Error; err != nil {
					tx.Rollback()
					return
				}
			}
			tx.Commit()
		}()
	}
	return c.JSON(http.StatusOK, common.SuccessPayLoadToMessage(i18n.DefaultLang.Load(i18n.INFO_ORDER_IS_DELETE)))
}
