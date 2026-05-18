package routing

import (
	"context"

	"gorm.io/gorm"
)

// Plugin is a GORM plugin that records the read-routing intent (primary vs
// replica) on Statement.Context before every Query/Raw callback. ConnPool
// reads it at SQL-call time; the metrics plugin reads it post-query to emit
// pool:<primary|replica>.
//
// The plugin records intent — not whether the replica pool is actually
// wired up — so phase-1 of the rollout (feature flag off, no real replica)
// still produces metrics that show how much traffic *would* go to the
// replica once enabled. ConnPool decides the final route by checking
// whether a replica pool exists.
//
// ForceACL flips the Allow list into a force-to-replica list: when true,
// any query whose primary table appears in ACL.Allow auto-routes to the
// replica without the caller having to opt in (no middleware, no scope,
// no WithReplica). This is the per-table promotion mechanism used during
// the rollout — turn it on once a table has been validated.
type Plugin struct {
	ACL      *TableACL
	ForceACL bool
}

var _ gorm.Plugin = (*Plugin)(nil)

func (p *Plugin) Name() string { return "routing" }

func (p *Plugin) Initialize(db *gorm.DB) error {
	if err := db.Callback().Query().Before("*").Register("routing:decide", p.decide); err != nil {
		return err
	}
	if err := db.Callback().Raw().Before("*").Register("routing:decide", p.decide); err != nil {
		return err
	}
	return nil
}

func (p *Plugin) decide(tx *gorm.DB) {
	if tx.Statement == nil {
		return
	}
	ctx := tx.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	tx.Statement.Context = WithDecision(ctx, p.choose(ctx, tx.Statement))
}

func (p *Plugin) choose(ctx context.Context, stmt *gorm.Statement) Pool {
	// Forced replica path (routing.Replica / routing.ReplicaScope /
	// WithForceReplica) bypasses everything except the absence of a
	// configured replica pool, which ConnPool handles by falling back to
	// primary. ACL doesn't apply here: the caller has taken explicit
	// responsibility for the routing decision at the call site.
	if IsForceReplica(ctx) {
		return PoolReplica
	}
	if IsWithoutReplica(ctx) {
		return PoolPrimary
	}
	// Deny / not-allow-listed always wins, regardless of ForceACL. The
	// join check below guards against an allow-listed primary table
	// dragging a denied table along through a JOIN.
	if !p.ACL.AllowsReplica(stmt.Table) {
		return PoolPrimary
	}
	for _, t := range joinTables(stmt) {
		if !p.ACL.AllowsReplica(t) {
			return PoolPrimary
		}
	}
	// ForceACL promotes the query to replica when the primary table has
	// been explicitly added to the Allow list — caller doesn't need to
	// opt in. The Deny / join checks above still apply, so a table can't
	// be promoted past a write-recency exclusion. An empty Allow list is
	// intentionally a no-op here: ForceACL requires the operator to
	// enumerate which tables are safe.
	if p.ForceACL && p.ACL.IsExplicitlyAllowed(stmt.Table) {
		return PoolReplica
	}
	if !UseReplica(ctx) {
		return PoolPrimary
	}
	return PoolReplica
}

// joinTables resolves Statement.Joins to underlying table names. .Joins(...)
// accepts either a relation field name ("Org" → resolved via the model's
// Relationships) or a raw SQL fragment ("LEFT JOIN orgs ON ..."). For the
// raw form we return the Name unchanged so callers can match on the bare
// table name when it appears as the Join's Name; multi-token SQL fragments
// won't match any ACL entry and are conservatively treated as allowed —
// callers needing tight control over raw SQL joins should use
// routing.WithoutReplica at the call site instead.
func joinTables(stmt *gorm.Statement) []string {
	if len(stmt.Joins) == 0 {
		return nil
	}
	out := make([]string, 0, len(stmt.Joins))
	for _, j := range stmt.Joins {
		out = append(out, resolveJoinTable(stmt, j.Name))
	}
	return out
}

func resolveJoinTable(stmt *gorm.Statement, name string) string {
	if stmt.Schema != nil {
		if rel, ok := stmt.Schema.Relationships.Relations[name]; ok && rel.FieldSchema != nil {
			return rel.FieldSchema.Table
		}
	}
	return name
}
