package routing

import (
	"context"

	"gorm.io/gorm"
)

// Replica returns a *gorm.DB session whose read queries are routed to the
// replica (subject to the table ACL and replica availability). Prefer this
// for worker/background code that doesn't pass through the GET-routing HTTP
// middleware, e.g.:
//
//	routing.Replica(db).Find(&rows)
func Replica(db *gorm.DB) *gorm.DB {
	return db.WithContext(WithReplica(stmtContext(db)))
}

// Primary returns a *gorm.DB session whose read queries are forced to the
// primary, overriding any upstream opt-in (request middleware, callers that
// set WithReplica). Use for read-after-write within the same request.
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
	db.Statement.Context = WithReplica(stmtContext(db))
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
