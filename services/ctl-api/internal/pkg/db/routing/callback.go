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
type Plugin struct {
	ACL *TableACL
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
	tx.Statement.Context = WithDecision(ctx, p.choose(ctx, tx.Statement.Table))
}

func (p *Plugin) choose(ctx context.Context, table string) Pool {
	if IsWithoutReplica(ctx) {
		return PoolPrimary
	}
	if !UseReplica(ctx) {
		return PoolPrimary
	}
	if !p.ACL.AllowsReplica(table) {
		return PoolPrimary
	}
	return PoolReplica
}
