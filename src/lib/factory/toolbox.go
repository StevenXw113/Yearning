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

package factory

import (
	"Yearning-go/src/engine"
	"Yearning-go/src/model"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cookieY/yee/logger"
	"github.com/vmihailenco/msgpack/v5"
	"math"
	"slices"
	"strconv"
	"time"

	"engine/ruleset"
	"gorm.io/gorm"
)

// RemoveString 从给定字符串切片中删除所有与指定字符串匹配的元素。
func RemoveString(s []string, p string) []string {
	return slices.DeleteFunc(slices.Clone(s), func(item string) bool { return item == p })
}

func Paging(page interface{}, total int) (start int, end int) {
	var i int
	switch v := page.(type) {
	case string:
		i, _ = strconv.Atoi(v)
	case int:
		i = v
	}
	start = i*total - total
	end = total
	return
}

// shortIdAlphabet 去掉了容易看混的字符（0/O、1/l/I），便于口头转述与手工输入。
const shortIdAlphabet = "23456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ"

// GenShortId 生成 n 位随机短 ID（56 字符表）。工单号、数据源 ID 这类需要人工转述的标识都用它。
func GenShortId(n int) string {
	b := make([]byte, n)
	if _, err := crand.Read(b); err != nil {
		// 几乎不可能发生；真发生时退回时间戳，保证调用方拿到非空 ID
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	for i, v := range b {
		b[i] = shortIdAlphabet[int(v)%len(shortIdAlphabet)]
	}
	return string(b)
}

// NextSourceId 生成不与既有数据源重复的短 ID：4 位起步，撞多了（或被用尽）自动加长。
// 必须查重——source_id 是权限校验与工单归属的依据，重复会导致跨数据源串数据。
func NextSourceId() string {
	for n := 4; n <= 8; n++ {
		for i := 0; i < 32; i++ {
			id := GenShortId(n)
			var cnt int64
			model.DB().Model(model.CoreDataSource{}).Where("source_id = ?", id).Count(&cnt)
			if cnt == 0 {
				return id
			}
		}
	}
	return GenShortId(16) // 兜底：极端情况下不阻塞建数据源
}

// OrderNo 生成工单编号：直接取工单的自增 id，唯一且便于转述。
// 项目级子工单在项目号后追加序号，形如 123-1、123-2；项目号本身用 seq=0。
func OrderNo(id uint, seq int) string {
	no := strconv.FormatUint(uint64(id), 10)
	if seq > 0 {
		no += "-" + strconv.Itoa(seq)
	}
	return no
}

func TimeDifference(t string) bool {
	if t == "" {
		return false
	}
	dt, _ := time.ParseInLocation("2006-01-02 15:04 ", t, time.Local)
	source := time.Now()
	// 单次读取快照，避免两次读取之间配置被热更新导致判断不一致
	exQueryTime := model.GloOther.Load().ExQueryTime
	if math.Abs(source.Sub(dt).Minutes()) > float64(exQueryTime) && float64(exQueryTime) > 0 {
		return true
	}
	return false
}

func JsonStringify(i interface{}) []byte {
	o, _ := json.Marshal(i)
	return o
}

func EmptyGroup() []byte {
	group, _ := json.Marshal([]string{})
	return group
}

func MapOn(l []string) map[string]struct{} {
	mp := make(map[string]struct{})
	for _, i := range l {
		mp[i] = struct{}{}
	}
	return mp
}

func ToJson(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func ToMsg(v interface{}) []byte {
	b, err := msgpack.Marshal(v)
	if err != nil {
		logger.DefaultLogger.Error(err)
	}
	return b
}

func CheckDataSourceRule(ruleId int) (*engine.AuditRole, error) {
	if ruleId != 0 {
		var r model.CoreRules
		var rule engine.AuditRole
		model.DB().Where("id = ?", ruleId).First(&r)
		if err := r.AuditRole.UnmarshalToJSON(&rule); err != nil {
			return nil, err
		}
		return &rule, nil
	}
	return model.GloRole.Load(), nil
}

// CheckDataSourceRuleSet 返回数据源生效的规则。
//
// RuleId != 0 时只读该规则集：优先使用 v2 Definition，不回退、不合并全局规则。
// MongoDB 的旧 AuditRole 会被转换为 v2 Definition；其他数据源继续返回旧 AuditRole。
// RuleId == 0 保持存量兼容，回退全局规则。
func CheckDataSourceRuleSet(source *model.CoreDataSource) (*ruleset.Definition, *engine.AuditRole, error) {
	if source == nil {
		return nil, nil, errors.New("数据源为空，无法读取规则集")
	}
	if source.RuleId == 0 {
		return nil, model.GloRole.Load(), nil
	}

	var r model.CoreRules
	err := model.DB().Model(model.CoreRules{}).Where("id = ?", source.RuleId).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, fmt.Errorf("数据源规则集 #%d 不存在", source.RuleId)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("读取数据源规则集 #%d 失败: %w", source.RuleId, err)
	}

	if len(r.Definition) > 0 {
		def, err := ruleset.Decode(r.Definition)
		if err != nil {
			return nil, nil, fmt.Errorf("解析数据源规则集 #%d 失败: %w", source.RuleId, err)
		}
		if source.DBType == model.DBTypeMongoDB {
			if err := ruleset.ValidateMongoDefinition(def); err != nil {
				return nil, nil, fmt.Errorf("数据源规则集 #%d 非法: %w", source.RuleId, err)
			}
			return &def, nil, nil
		}
		return nil, nil, fmt.Errorf("数据源类型 #%d 暂不支持 v2 规则集 #%d", source.DBType, source.RuleId)
	}

	var old engine.AuditRole
	if err := r.AuditRole.UnmarshalToJSON(&old); err != nil {
		return nil, nil, fmt.Errorf("解析数据源规则集 #%d 失败: %w", source.RuleId, err)
	}
	if source.DBType == model.DBTypeMongoDB {
		def := ruleset.MongoDefinitionFromAPI(engine.AuditRoleToAPI(&old))
		if err := ruleset.ValidateMongoDefinition(def); err != nil {
			return nil, nil, fmt.Errorf("数据源规则集 #%d 非法: %w", source.RuleId, err)
		}
		return &def, &old, nil
	}
	return nil, &old, nil
}
