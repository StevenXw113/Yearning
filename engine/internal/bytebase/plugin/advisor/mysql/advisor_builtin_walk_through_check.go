package mysql

import (
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/advisor"
)

func init() {
	advisor.Register(storepb.Engine_MYSQL, storepb.SQLReviewRule_BUILTIN_WALK_THROUGH_CHECK, &advisor.BuiltinWalkThroughCheckAdvisor{})
	advisor.Register(storepb.Engine_MARIADB, storepb.SQLReviewRule_BUILTIN_WALK_THROUGH_CHECK, &advisor.BuiltinWalkThroughCheckAdvisor{})
	advisor.Register(storepb.Engine_OCEANBASE, storepb.SQLReviewRule_BUILTIN_WALK_THROUGH_CHECK, &advisor.BuiltinWalkThroughCheckAdvisor{})
}
