package routing

import "context"

// Pool identifies which database pool a query was (or will be) routed to.
type Pool string

const (
	PoolPrimary Pool = "primary"
	PoolReplica Pool = "replica"
)

type (
	replicaOptInKey  struct{}
	replicaOptOutKey struct{}
	replicaForceKey  struct{}
	decisionKey      struct{}
)

// WithReplica marks the context to opt-in to replica routing for read queries.
// The actual routing decision is made by the routing GORM callback, which also
// considers any opt-out (WithoutReplica), the table ACL, and whether a replica
// is configured.
func WithReplica(ctx context.Context) context.Context {
	return context.WithValue(ctx, replicaOptInKey{}, true)
}

// UseReplica reports whether the context has opted in to replica routing.
func UseReplica(ctx context.Context) bool {
	v, _ := ctx.Value(replicaOptInKey{}).(bool)
	return v
}

// WithoutReplica marks the context as opted-out of replica routing, even if a
// caller higher up the stack has opted in (e.g. the GET middleware). Use for
// read-after-write inside a request or any code path that must observe its
// own writes immediately.
func WithoutReplica(ctx context.Context) context.Context {
	return context.WithValue(ctx, replicaOptOutKey{}, true)
}

// IsWithoutReplica reports whether the context has been forced to the primary.
func IsWithoutReplica(ctx context.Context) bool {
	v, _ := ctx.Value(replicaOptOutKey{}).(bool)
	return v
}

// WithForceReplica marks the context as an unconditional replica-read. It
// bypasses the table ACL and overrides WithoutReplica — use only when the
// caller is certain the query can run on the replica (most often via the
// routing.Replica / routing.ReplicaScope helpers).
//
// The only thing that downgrades this back to primary is the absence of a
// configured replica pool: ConnPool will silently fall back when no
// replica *sql.DB is wired up (i.e. when DBReplicaEnabled is false or
// DBReplicaHost is empty).
func WithForceReplica(ctx context.Context) context.Context {
	return context.WithValue(ctx, replicaForceKey{}, true)
}

// IsForceReplica reports whether the context has been marked for a forced
// replica read.
func IsForceReplica(ctx context.Context) bool {
	v, _ := ctx.Value(replicaForceKey{}).(bool)
	return v
}

// WithDecision records the routing decision for the current query so that the
// ConnPool and the metrics plugin observe the same value.
func WithDecision(ctx context.Context, p Pool) context.Context {
	return context.WithValue(ctx, decisionKey{}, p)
}

// DecisionFromContext returns the recorded routing decision. Defaults to
// PoolPrimary when no decision has been made — that matches ConnPool's
// fallback when the context hasn't been marked for replica use.
func DecisionFromContext(ctx context.Context) Pool {
	if v, ok := ctx.Value(decisionKey{}).(Pool); ok {
		return v
	}
	return PoolPrimary
}
