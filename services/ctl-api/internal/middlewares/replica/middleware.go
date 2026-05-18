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
