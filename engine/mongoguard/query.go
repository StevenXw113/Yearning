package mongoguard

import (
	"errors"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// The query allowlist is independent of rule sets and applies while the audit
// engine is unavailable. Instance-wide database enumeration stays excluded.
var readOnlyCommands = map[string]struct{}{
	"find": {}, "aggregate": {}, "count": {}, "distinct": {}, "explain": {},
	"getmore": {}, "killcursors": {},
	"listcollections": {}, "listindexes": {}, "collstats": {}, "dbstats": {}, "indexstats": {},
	"ping": {}, "hello": {}, "ismaster": {}, "buildinfo": {},
	"serverstatus": {}, "hostinfo": {}, "connectionstatus": {}, "whatsmyuri": {}, "getparameter": {},
}

// ValidateQuery checks the command and returns the document to audit. For
// explain this is the wrapped read command. The caller still executes cmd.
func ValidateQuery(cmd bson.D) (bson.D, error) {
	if len(cmd) == 0 {
		return nil, errors.New("MongoDB 命令不能为空")
	}
	query := cmd
	if strings.EqualFold(cmd[0].Key, "explain") {
		inner, ok := cmd[0].Value.(bson.D)
		if !ok || len(inner) == 0 || strings.EqualFold(inner[0].Key, "explain") {
			return nil, errors.New("explain 必须包含一条只读命令")
		}
		query = inner
	}
	name := strings.ToLower(query[0].Key)
	if _, ok := readOnlyCommands[name]; !ok {
		return nil, fmt.Errorf("查询页只能执行白名单中的只读命令，%q 不受支持", query[0].Key)
	}
	if name == "aggregate" {
		for _, field := range query[1:] {
			if !strings.EqualFold(field.Key, "pipeline") {
				continue
			}
			stages, _ := field.Value.(bson.A)
			for _, item := range stages {
				stage, _ := item.(bson.D)
				for _, op := range stage {
					if strings.EqualFold(op.Key, "$out") || strings.EqualFold(op.Key, "$merge") {
						return nil, errors.New("查询页不允许 aggregate 的 $out / $merge 写入阶段")
					}
				}
			}
		}
	}
	if op := scriptOperator(cmd); op != "" {
		return nil, fmt.Errorf("查询页禁止使用 %s（在服务端执行脚本）", op)
	}
	return query, nil
}

func scriptOperator(value any) string {
	switch value := value.(type) {
	case bson.D:
		for _, field := range value {
			switch strings.ToLower(field.Key) {
			case "$where", "$function", "$accumulator":
				return field.Key
			}
			if op := scriptOperator(field.Value); op != "" {
				return op
			}
		}
	case bson.A:
		for _, item := range value {
			if op := scriptOperator(item); op != "" {
				return op
			}
		}
	}
	return ""
}
