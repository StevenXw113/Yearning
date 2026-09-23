package common

import (
	"Yearning-go/src/lib/permission"
	"gorm.io/gorm"
	"reflect"
)

// QueryField 工单列表字段。id 用于把项目的子工单按创建顺序排列；
// batch_id 供前端按项目（批次）聚合展示：先看项目，展开后逐条查看/审核。
const QueryField = "id, work_id, username, text, backup, date, real_name, `status`, `type`, `delay`, `source`,`id_c`,`data_base`,`table`,`execute_time`,source_id,assigned,current_step,relevant,`file`,batch_id"

func AccordingToWorkId(workId string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if workId == "" {
			return db
		}
		// 工单号已改为自增 id（项目子工单形如 123-1）：按「精确命中 or 该项目的全部子工单」查。
		// 数字编号用 like 子串匹配会命中大量无关工单（搜 1 命中所有含 1 的编号）。
		return db.Where("work_id = ? OR work_id LIKE ?", workId, workId+"-%")
	}
}

func AccordingToQueryPer() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("`status` in (?)", []int{1, 3})
	}
}

func AccordingToAllQueryOrderState(state int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch state {
		case 7:
			return db
		default:
			return db.Where("`status` = ?", state)
		}
	}
}

func AccordingToOrderState() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("`status` in (?)", []int{1, 4, 0})
	}
}

func AccordingToAllOrderState(state int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch state {
		case 8:
			return db
		default:
			return db.Where("`status` = ?", state)
		}
	}
}

// AccordingToSource 按数据源名称过滤（表头数据源筛选）。
func AccordingToSource(source string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if source == "" {
			return db
		}
		return db.Where("`source` = ?", source)
	}
}

func AccordingToAllOrderType(state int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		switch state {
		case 2:
			return db
		default:
			return db.Where("`type` = ?", state)
		}
	}
}

func AccordingToAssigned(user string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("`assigned` like ?", "%"+user+"%")
	}
}

// AccordingQueryToAssigned 把查询工单限定为「当前审批人」名下的。
// 审计员（isRecord）与内置超管不受此限：他们需要查看全部查询工单/查询记录。
func AccordingQueryToAssigned(isRecord bool, username string) func(db *gorm.DB) *gorm.DB {

	return func(db *gorm.DB) *gorm.DB {
		if isRecord || permission.IsSuperUser(username) {
			return db
		}
		return db.Where("`assigned` like ?", "%"+username+"%")
	}
}

func AccordingToUsername(user string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if user == "" {
			return db
		}
		return db.Where("username like ?", "%"+user+"%")
	}
}

func AccordingToPrincipal(principal string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if principal == "" {
			return db
		}
		return db.Where("principal like ?", "%"+principal+"%")
	}
}

func AccordingToRealName(user string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if user == "" {
			return db
		}
		return db.Where("real_name like ?", "%"+user+"%")
	}
}

func AccordingToMail(user string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if user == "" {
			return db
		}
		return db.Where("email like ?", "%"+user+"%")
	}
}

func AccordingToDate(time []string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if reflect.DeepEqual(time, []string{"", ""}) || len(time) != 2 {
			return db
		}
		return db.Where("date >= ? AND date <= ?", time[0], time[1])
	}
}

func AccordingToRelevant(user string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {

		return db.Where("JSON_SEARCH(relevant, 'all', ?) IS NOT NULL", user)
	}
}

func AccordingToUsernameEqual(user string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if user == "" {
			return db
		}
		return db.Where("username = ?", user)
	}
}

func AccordingToIDEqual(id int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("`id` = ?", id)
	}
}

func AccordingToText(text string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if text == "" {
			return db
		}
		return db.Where("text like ?", "%"+text+"%")
	}
}

func AccordingToOrderName(text string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if text == "" {
			return db
		}
		return db.Where("`name` like ?", "%"+text+"%")
	}
}

func AccordingToOrderIDC(text string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if text == "" {
			return db
		}
		return db.Where("id_c LIKE ? ", "%"+text+"%")
	}
}

func AccordingToOrderAccurateIDC(text string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if text == "" {
			return db
		}
		return db.Where("id_c = ? ", text)
	}
}

func AccordingToOrderIP(text string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if text == "" {
			return db
		}
		return db.Where("ip LIKE ? ", "%"+text+"%")
	}
}

func AccordingToOrderSource(text string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if text == "" {
			return db
		}
		return db.Where("source LIKE ? ", "%"+text+"%")
	}
}

func AccordingToOrderType(text int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if text == -1 {
			return db
		}
		return db.Where("`is_query` = ?", text)
	}
}

func AccordingToOrderDept(text string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if text == "" {
			return db
		}
		return db.Where("department LIKE ?", "%"+text+"%")
	}
}

func AccordingToGroupSourceIsQuery(start, end int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("is_query =? or is_query = ?", start, end)
	}
}

func AccordingToGroupNameIsLike(text string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if text == "" {
			return db
		}
		return db.Where("`group` like ?", "%"+text+"%")
	}
}

func AccordingToSchemaNotIn(isSchema bool, excludeDbList []string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if len(excludeDbList) == 0 {
			return db
		}
		if isSchema {
			return db.Where("SCHEMA_NAME not in (?)", excludeDbList)
		}
		return db.Where("table_schema not in (?)", excludeDbList)
	}
}

func AccordingToSchemaIn(includeDbList string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("table_schema = ? ", includeDbList)
	}
}
