package scopes

import (
	"context"

	"gorm.io/gorm"

	"github.com/nuonco/nuon/services/ctl-api/internal/pkg/db/routing"
)

// WithReplica is a GORM scope that forces this query chain to read from
// the read replica when one is configured. It bypasses the table ACL —
// the caller has taken explicit responsibility for the routing decision
// at the call site. ConnPool transparently falls back to primary if no
// replica pool is wired up (DBReplicaEnabled off / DBReplicaHost empty).
//
//	s.db.WithContext(ctx).Scopes(scopes.WithReplica).Find(&rows)
func WithReplica(db *gorm.DB) *gorm.DB {
	db.Statement.Context = routing.WithForceReplica(stmtContext(db))
	return db
}

// WithoutReplica is a GORM scope that forces this query chain to the
// primary, overriding any upstream opt-in (the global GET middleware,
// per-route OptIn, or a wrapping WithReplica). Use for read-after-write
// inside a single request.
//
//	s.db.WithContext(ctx).Scopes(scopes.WithoutReplica).First(&row)
func WithoutReplica(db *gorm.DB) *gorm.DB {
	db.Statement.Context = routing.WithoutReplica(stmtContext(db))
	return db
}

func stmtContext(db *gorm.DB) context.Context {
	if db.Statement != nil && db.Statement.Context != nil {
		return db.Statement.Context
	}
	return context.Background()
}
