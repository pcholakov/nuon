package psql

import (
	"gorm.io/gorm"

	"github.com/nuonco/nuon/services/ctl-api/internal/app"
	"github.com/nuonco/nuon/services/ctl-api/internal/pkg/db/routing"
)

// replicaACL is the per-table allow/deny configuration for read-replica
// routing. Allow entries are tables that have been validated for replica
// reads — they're eligible for auto-promotion when DBReplicaForceACL is
// on. With ForceACL off the list is dormant; opt-in paths (middleware,
// scope, @replica-read, WithReplica) still work for every non-denied
// table.
//
// Add models here as they get validated against replication lag and any
// read-after-write hot paths get audited. Move a model into Deny if a
// regression shows up — Deny is the fast rollback for a single table.
func replicaACL(db *gorm.DB) *routing.TableACL {
	return routing.NewACLBuilder(db).
		Allow(
			&app.Org{},
			&app.App{},
			&app.Install{},
		).
		Build()
}
