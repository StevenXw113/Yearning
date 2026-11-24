// Package mongoguard provides command parsing and the query execution boundary
// shared by the Yearning application and the audit engine.
package mongoguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Parse decodes exactly one Extended JSON object and preserves BSON field order
// and types. Duplicate document keys make policy checks ambiguous.
func Parse(text string) (bson.D, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("MongoDB 命令不能为空")
	}
	if text[0] != '{' || !json.Valid([]byte(text)) {
		return nil, errors.New("MongoDB 命令必须是单个合法的 JSON 对象")
	}
	var cmd bson.D
	if err := bson.UnmarshalExtJSON([]byte(text), false, &cmd); err != nil {
		return nil, fmt.Errorf("命令不是合法的 MongoDB 扩展 JSON：%w", err)
	}
	if len(cmd) == 0 || cmd[0].Key == "" {
		return nil, errors.New("无法识别 MongoDB 命令名")
	}
	return cmd, uniqueKeys(cmd)
}

func uniqueKeys(value any) error {
	switch value := value.(type) {
	case bson.D:
		seen := make(map[string]struct{}, len(value))
		for _, field := range value {
			if _, duplicate := seen[field.Key]; duplicate {
				return fmt.Errorf("MongoDB 命令包含重复字段 %q", field.Key)
			}
			seen[field.Key] = struct{}{}
			if err := uniqueKeys(field.Value); err != nil {
				return err
			}
		}
	case bson.A:
		for _, item := range value {
			if err := uniqueKeys(item); err != nil {
				return err
			}
		}
	case bson.CodeWithScope:
		return uniqueKeys(value.Scope)
	}
	return nil
}
