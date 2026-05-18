package replica

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/nuonco/nuon/services/ctl-api/internal/pkg/db/routing"
)

type middleware struct {
	excludedRoutes map[string]struct{}
}

func (m middleware) Name() string {
	return "replica"
}

// Handler marks GET requests so that read queries are routed to the replica
// database via the routing.ConnPool. Routes registered via Exclude are
// instead forced to the primary, overriding any upstream opt-in — use for
// endpoints that read-after-write within the same request, or that hit a
// table currently excluded from replica rollout.
func (m middleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
		}
		ctx := c.Request.Context()
		if _, excluded := m.excludedRoutes[c.FullPath()]; excluded {
			ctx = routing.WithoutReplica(ctx)
		} else {
			ctx = routing.WithReplica(ctx)
		}
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// New returns a replica middleware that opts every GET request into replica
// routing. Used by FX wiring — keep this signature parameter-free so it can
// be provided via AsMiddleware. Use NewWithExclusions when you need per-route
// opt-out.
func New() *middleware {
	return &middleware{}
}

// NewWithExclusions returns a replica middleware that opts GET requests into
// replica routing except for the listed route patterns. excludedRoutes are
// matched against gin's FullPath (the route pattern, e.g.
// "/v1/installs/:install_id") and forced back to the primary.
func NewWithExclusions(excludedRoutes ...string) *middleware {
	excluded := make(map[string]struct{}, len(excludedRoutes))
	for _, r := range excludedRoutes {
		excluded[r] = struct{}{}
	}
	return &middleware{excludedRoutes: excluded}
}

// OptIn is a per-route gin middleware that marks the request context for
// replica routing. Compose it onto a specific handler when you want a
// non-GET endpoint to hit the replica, or when relying on the global GET
// middleware isn't desirable:
//
//	api.POST("/v1/things/search", replica.OptIn(), s.SearchThings)
//	api.GET("/v1/things",         replica.OptIn(), s.ListThings)
//
// Safe to combine with the global GET middleware — applying OptIn on top of
// an already-opted-in request is a no-op.
func OptIn() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(routing.WithReplica(c.Request.Context()))
		c.Next()
	}
}

// OptOut is a per-route gin middleware that forces this request's reads to
// the primary, overriding any upstream opt-in (e.g. the global GET
// middleware). Use on endpoints that read-after-write within the same
// request:
//
//	api.GET("/v1/installs/:install_id", replica.OptOut(), s.GetInstall)
func OptOut() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request = c.Request.WithContext(routing.WithoutReplica(c.Request.Context()))
		c.Next()
	}
}
