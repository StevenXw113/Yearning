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

package service

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/model"
	_ "Yearning-go/src/model"
	"Yearning-go/src/router"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/cookieY/yee"
	"github.com/cookieY/yee/logger"
	"github.com/cookieY/yee/middleware"
	"net/http"
	"os"
)

//go:embed chat/*
var chatf embed.FS

//go:embed chat/server/app/index.html
var chatindex string

//go:embed dist/*
var f embed.FS

//go:embed dist/index.html
var html string

// loadDBInit 加载全局配置到进程内缓存。
// 解析失败必须中断启动：否则查询行数上限、审核规则等会静默变成零值。
func loadDBInit() error {
	if err := model.DB().First(&model.GloPer).Error; err != nil {
		return err
	}
	var message model.Message
	var other model.Other
	var role engine.AuditRole
	var ai model.AI
	if model.GloPer.Message != nil {
		if err := json.Unmarshal(model.GloPer.Message, &message); err != nil {
			return err
		}
	}
	if model.GloPer.Other != nil {
		if err := json.Unmarshal(model.GloPer.Other, &other); err != nil {
			return err
		}
	}
	if model.GloPer.AuditRole != nil {
		if err := json.Unmarshal(model.GloPer.AuditRole, &role); err != nil {
			return err
		}
	}
	if model.GloPer.AI != nil {
		if err := json.Unmarshal(model.GloPer.AI, &ai); err != nil {
			return err
		}
	}
	model.GloMessage.Store(&message)
	model.GloOther.Store(&other)
	model.GloRole.Store(&role)
	model.GloAI.Store(&ai)
	return nil
}

func StartYearning(port string) {
	if err := loadDBInit(); err != nil {
		logger.DefaultLogger.Errorf("全局配置加载失败: %v", err)
		os.Exit(1)
		return
	}
	go cronTabMaskQuery()
	go cronTabTotalTickets()
	go cronTabDelayOrder()
	e := yee.New()
	e.Pack("/front", f, "dist")
	e.Pack("/_next", chatf, "chat")
	e.Use(middleware.Cors())
	e.Use(middleware.Logger())
	e.Use(middleware.Secure())
	e.Use(middleware.Recovery())
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 9,
	}))
	e.SetLogLevel(model.TransferLogLevel())
	e.GET("/chatbot", func(c yee.Context) error {
		return c.HTML(http.StatusOK, chatindex)
	})
	e.GET("/", func(c yee.Context) error {
		return c.HTML(http.StatusOK, html)
	})
	router.AddRouter(e)
	fmt.Println("Yearning is running on port: ", port)
	e.Run(fmt.Sprintf(":%s", port))
}
