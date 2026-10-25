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
	"Yearning-go/src/router"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/cookieY/yee"
	"github.com/cookieY/yee/logger"
	"github.com/cookieY/yee/middleware"
	"net/http"
	"os"
	"strings"
)

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
	// 单实例互斥（默认开启）：cron 统计、OSC 中止、执行自愈等均假设单实例。
	// 抢不到锁说明另一实例在跑，直接拒绝启动，由运维排查。
	if !model.C.General.DisableInstanceLock {
		if err := AcquireInstanceLock(context.Background()); err != nil {
			logger.DefaultLogger.Errorf("单实例互斥: %v", err)
			os.Exit(1)
			return
		}
		logger.DefaultLogger.Info("已获取单实例锁（MySQL GET_LOCK）")
	}
	go cronTabMaskQuery()
	go cronTabTotalTickets()
	go cronTabDelayOrder()
	// 僵尸工单自愈：依赖单实例互斥（上方 AcquireInstanceLock 已先行完成），
	// 多实例场景该 cron 会误杀其他实例正在执行的工单
	go cronTabSelfHeal()
	// 历史归档（ArchiveDays>0 时生效）：同样依赖单实例互斥（双实例会重复
	// 迁移同一批工单，DELETE 幂等但会互相干扰批次计数）
	go cronTabArchive()
	e := yee.New()
	e.Pack("/front", f, "dist")
	// CORS 收紧：配置了 CorsOrigins 就只放行这些来源（同源部署不受影响——
	// 同源请求不触发 CORS）；留空保持通配 * 兼容存量部署。
	if origins := strings.Split(strings.TrimSpace(model.C.General.CorsOrigins), ","); len(origins) > 0 && origins[0] != "" {
		cleaned := origins[:0]
		for _, o := range origins {
			if o = strings.TrimSpace(o); o != "" {
				cleaned = append(cleaned, o)
			}
		}
		if len(cleaned) > 0 {
			e.Use(middleware.CorsWithConfig(middleware.CORSConfig{Origins: cleaned}))
		} else {
			e.Use(middleware.Cors())
		}
	} else {
		e.Use(middleware.Cors())
	}
	e.Use(middleware.Logger())
	e.Use(middleware.Secure())
	e.Use(middleware.Recovery())
	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		Level: 9,
	}))
	e.SetLogLevel(model.TransferLogLevel())
	e.GET("/", func(c yee.Context) error {
		return c.HTML(http.StatusOK, html)
	})
	router.AddRouter(e)
	fmt.Println("Yearning is running on port: ", port)
	e.Run(fmt.Sprintf(":%s", port))
}
