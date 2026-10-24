package permission

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// SuperUser 内置超级管理员用户名：始终拥有全部权限，与配置无关。
const SuperUser = "admin"

// superUsers 解析后的超管名单（惰性：首次调用 IsSuperUser 时从
// [General].SuperUsers 解析一次——配置进程内不热更，原子替换保并发读）。
// 不在 model.initConfig 里回调注入：permission 已依赖 model，反向调用成环。
var (
	superUsers     atomic.Value // map[string]struct{}
	superUsersOnce sync.Once
)

func superUserSet() map[string]struct{} {
	superUsersOnce.Do(func() {
		list := map[string]struct{}{SuperUser: {}}
		for _, name := range strings.Split(model.C.General.SuperUsers, ",") {
			if name = strings.TrimSpace(name); name != "" {
				list[name] = struct{}{}
			}
		}
		superUsers.Store(list)
	})
	v, _ := superUsers.Load().(map[string]struct{})
	return v
}

// IsSuperUser 判断是否为超级管理员：内置 admin + [General].SuperUsers 配置名单。
// 此前硬编码单账号 admin，多管理员/管理分层无法表达。
func IsSuperUser(user string) bool {
	_, found := superUserSet()[user]
	return found
}

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

// permCacheTTL 权限列表缓存有效期。权限查询在每请求/每条查询 WS 消息上发生，
// 原实现每次 N+1 查库（grained → 每权限组 → admin 全量数据源）；TTL 内同用户
// 只算一次。变更点另配主动失效（InvalidateAll），TTL 只是漏埋时的兜底。
const permCacheTTL = 30 * time.Second

type permCacheEntry struct {
	list     *model.PermissionList
	expireAt time.Time
}

var permCache struct {
	sync.Mutex
	m map[string]permCacheEntry
}

func init() {
	permCache.m = map[string]permCacheEntry{}
}

// InvalidateAll 清空权限缓存。权限组/用户组分配/数据源/用户的写路径在
// 落库成功后调用，让变更立即生效（不等 TTL 过期）。
func InvalidateAll() {
	permCache.Lock()
	permCache.m = map[string]permCacheEntry{}
	permCache.Unlock()
}

func (service *PermissionService) cachedOrLoad(user string) *model.PermissionList {
	now := time.Now()
	permCache.Lock()
	if e, ok := permCache.m[user]; ok && now.Before(e.expireAt) {
		permCache.Unlock()
		return e.list
	}
	permCache.Unlock()

	list := service.loadPermissionList(user)

	permCache.Lock()
	permCache.m[user] = permCacheEntry{list: list, expireAt: now.Add(permCacheTTL)}
	permCache.Unlock()
	return list
}

// loadPermissionList 实际查库组装权限列表（原 CreatePermissionList 主体）。
func (service *PermissionService) loadPermissionList(user string) *model.PermissionList {
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

// CreatePermissionList 返回用户的数据源权限列表（带 TTL 缓存，见 permCacheTTL）。
func (service *PermissionService) CreatePermissionList(user string) *model.PermissionList {
	return service.cachedOrLoad(user)
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
