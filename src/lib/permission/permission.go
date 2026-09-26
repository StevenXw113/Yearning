package permission

import (
	"Yearning-go/src/lib/vars"
	"Yearning-go/src/model"
	"gorm.io/gorm"
)

// PermissionService 提供权限相关的服务
type PermissionService struct {
	db *gorm.DB
}

type Control struct {
	User     string
	Kind     int
	WorkId   string
	SourceId string
}

// NewPermissionService 创建一个新的权限服务实例
func NewPermissionService(db *gorm.DB) *PermissionService {
	return &PermissionService{db: db}
}

// SuperUser 内置超级管理员用户名：拥有全部数据源权限，不受权限组配置约束。
// 与 router.SuperManageGroup 判定管理端权限用的是同一个约定。
const SuperUser = "admin"

// IsSuperUser 判断是否为内置超级管理员
func IsSuperUser(user string) bool { return user == SuperUser }

// CreatePermissionListFromGroups 从组ID列表创建一个合并的权限列表
func (service *PermissionService) CreatePermissionListFromGroups(groupIDs []string) *model.PermissionList {
	combinedPermissions := new(model.PermissionList)

	for _, groupID := range groupIDs {
		var coreRoleGroup model.CoreRoleGroup
		var permissions model.PermissionList

		// 查询数据库中的角色组
		if err := service.db.Where("group_id = ?", groupID).First(&coreRoleGroup).Error; err != nil {
			model.DefaultLogger.Errorf("Error fetching group with ID %s: %v", groupID, err)
			continue
		}

		// 将权限数据解码到权限列表中
		if err := coreRoleGroup.Permissions.UnmarshalToJSON(&permissions); err != nil {
			model.DefaultLogger.Errorf("Error unmarshalling permissions for group ID %s: %v", groupID, err)
			continue
		}

		// 合并权限列表
		appendPermissions(combinedPermissions, &permissions)
	}

	// 去除重复的权限
	removeDuplicatePermissions(combinedPermissions)

	return combinedPermissions
}

// CreatePermissionList 返回用户的数据源权限列表。超级管理员直接取全部数据源，
// 无需加入任何权限组；其余用户按权限组合并（无权限组时返回空列表）。
func (service *PermissionService) CreatePermissionList(user string) *model.PermissionList {
	if IsSuperUser(user) {
		var ids []string
		service.db.Model(model.CoreDataSource{}).Pluck("source_id", &ids)
		return &model.PermissionList{DDLSource: ids, DMLSource: ids, QuerySource: ids}
	}
	var grained model.CoreGrained
	var roleGroups []string
	// 查询数据库中的精细权限
	if err := service.db.Model(model.CoreGrained{}).Where("username = ?", user).First(&grained).Error; err != nil {
		model.DefaultLogger.Infof("Error fetching grained permissions for user %s: %v", user, err)
		return new(model.PermissionList)
	}
	// 解码组信息
	if err := grained.Group.UnmarshalToJSON(&roleGroups); err != nil {
		model.DefaultLogger.Errorf("Error unmarshalling group information for user %s: %v", user, err)
		return new(model.PermissionList)
	}
	return service.CreatePermissionListFromGroups(roleGroups)
}

// Equal 检查用户对资源的权限
func (service *PermissionService) Equal(control *Control) bool {
	permissions := service.CreatePermissionList(control.User)
	// 检查权限
	switch control.Kind {
	case vars.DDL:
		return contains(permissions.DDLSource, control.SourceId)
	case vars.DML:
		return contains(permissions.DMLSource, control.SourceId)
	case vars.QUERY:
		return contains(permissions.QuerySource, control.SourceId)
	}

	return false
}

// contains 判断切片里是否含某个值（数据源数量在几十以内，线性查找足够）
func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// appendPermissions 将源权限列表中的权限追加到目标权限列表中
func appendPermissions(target, source *model.PermissionList) {
	target.DDLSource = append(target.DDLSource, source.DDLSource...)
	target.DMLSource = append(target.DMLSource, source.DMLSource...)
	target.QuerySource = append(target.QuerySource, source.QuerySource...)
}

// removeDuplicatePermissions 移除权限列表中的重复项
func removeDuplicatePermissions(permissionList *model.PermissionList) {
	permissionList.DDLSource = unique(permissionList.DDLSource)
	permissionList.DMLSource = unique(permissionList.DMLSource)
	permissionList.QuerySource = unique(permissionList.QuerySource)
}

// unique 保序去重
func unique(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
