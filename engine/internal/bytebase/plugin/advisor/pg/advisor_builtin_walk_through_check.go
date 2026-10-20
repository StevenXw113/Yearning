package pg

import (
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/advisor"
)

func init() {
	advisor.Register(storepb.Engine_POSTGRES, storepb.SQLReviewRule_BUILTIN_WALK_THROUGH_CHECK, &advisor.BuiltinWalkThroughCheckAdvisor{})
}
