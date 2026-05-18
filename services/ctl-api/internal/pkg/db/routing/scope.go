package routing

import (
	"context"

	"gorm.io/gorm"
)

// Replica returns a *gorm.DB session whose read queries are routed to the
// replica whenever a replica pool is configured (DBReplicaEnabled +
// DBReplicaHost). It is an explicit, alternative path to the request-level
// middleware: the table ACL does not apply because the caller is taking
// direct responsibility for the routing choice at the call site.
//
// Prefer this for worker/background code that doesn't pass through the
// GET-routing HTTP middleware, and for any handler that is certain a given
// query is safe to read from a replica:
//
//	routing.Replica(s.db).Find(&rows)
//
// If no replica is configured, ConnPool transparently falls back to
// primary — callers don't need to test for it.
func Replica(db *gorm.DB) *gorm.DB {
	return db.WithContext(WithForceReplica(stmtContext(db)))
}

// Primary returns a *gorm.DB session whose read queries are forced to the
// primary, overriding any upstream opt-in (request middleware, callers
// that set WithReplica). Use for read-after-write within the same request.
func Primary(db *gorm.DB) *gorm.DB {
	return db.WithContext(WithoutReplica(stmtContext(db)))
}

// ReplicaScope is the same as Replica but in the shape GORM scopes expect:
//
//	db.Scopes(routing.ReplicaScope).Find(&rows)
func ReplicaScope(db *gorm.DB) *gorm.DB {
	if db.Statement == nil {
		return Replica(db)
	}
	db.Statement.Context = WithForceReplica(stmtContext(db))
	return db
}

// PrimaryScope is the same as Primary but in the shape GORM scopes expect.
func PrimaryScope(db *gorm.DB) *gorm.DB {
	if db.Statement == nil {
		return Primary(db)
	}
	db.Statement.Context = WithoutReplica(stmtContext(db))
	return db
}

func stmtContext(db *gorm.DB) context.Context {
	if db.Statement != nil && db.Statement.Context != nil {
		return db.Statement.Context
	}
	return context.Background()
}
