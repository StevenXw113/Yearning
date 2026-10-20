package pg

import (
	storepb "engine/internal/bytebase/generated-go/store"
	"engine/internal/bytebase/plugin/parser/base"
)

func init() {
	base.RegisterQueryValidator(storepb.Engine_POSTGRES, validateQueryANTLR)
	// Redshift has its own implementation in the redshift package
	base.RegisterQueryValidator(storepb.Engine_COCKROACHDB, validateQueryANTLR)
}
