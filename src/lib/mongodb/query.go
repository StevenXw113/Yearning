package mongodb

import (
	"engine/mongoguard"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ValidateQuery enforces the same read-only policy as the audit engine.
func ValidateQuery(cmd bson.D) error {
	_, err := mongoguard.ValidateQuery(cmd)
	return err
}
