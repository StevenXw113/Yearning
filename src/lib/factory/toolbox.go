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
	"github.com/cookieY/yee/logger"
	"github.com/vmihailenco/msgpack/v5"
	"math"
	"strconv"
	"time"
)

const None = "none" //无延迟

// RemoveString 从给定字符串切片中删除所有与指定字符串匹配的元素。
func RemoveString(s []string, p string) []string {
	result := []string{} // 创建一个新的切片用于保存不匹配的元素

	for _, item := range s {
		if item != p {
			result = append(result, item) // 仅将不匹配的元素添加到结果中
		}
	}
	return result // 返回去掉匹配项后的新切片
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

// workIdAlphabet 去掉了容易看混的字符（0/O、1/l/I），便于口头转述与手工输入。
const workIdAlphabet = "23456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ"

// GenWorkId 生成 8 位随机工单号。
// 早先用 36 位 UUID：太长、不便于人工转述。8 位 × 56 字符表 ≈ 9.7e13 种组合，
// 十万级工单量下碰撞概率可忽略（且 work_id 在库里是普通索引，不强制唯一）。
func GenWorkId() string {
	b := make([]byte, 8)
	if _, err := crand.Read(b); err != nil {
		// 几乎不可能发生；真发生时退回时间戳，保证调用方拿到一个非空 ID
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	for i, v := range b {
		b[i] = workIdAlphabet[int(v)%len(workIdAlphabet)]
	}
	return string(b)
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
