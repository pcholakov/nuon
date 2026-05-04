package helpers

import (
	"context"
	"fmt"

	"go.temporal.io/sdk/workflow"

	"github.com/nuonco/nuon/services/ctl-api/internal/app"
	queueclient "github.com/nuonco/nuon/services/ctl-api/internal/pkg/queue/client"
	"github.com/nuonco/nuon/services/ctl-api/internal/pkg/queue/signal"
)

// connectionEventSignal is a minimal signal type that matches the
// connection_event.Signal type string. We define it here to avoid
// an import cycle (vcs/helpers cannot import vcs/signals/v2/connection_event
// because connection_event imports apps packages that depend on vcs/helpers).
type connectionEventSignal struct {
	VCSConnectionID string `json:"vcs_connection_id"`
	VCSEventID      string `json:"vcs_event_id"`
}

func (s *connectionEventSignal) Type() signal.SignalType           { return "vcs_connection_event" }
func (s *connectionEventSignal) Validate(_ workflow.Context) error { return nil }
func (s *connectionEventSignal) Execute(_ workflow.Context) error  { return nil }

// EnqueueVCSConnectionEvent enqueues a connection_event signal to the VCS connection's queue.
// This is called after a webhook event is received to trigger processing of the event.
func (h *Helpers) EnqueueVCSConnectionEvent(ctx context.Context, vcsConn *app.VCSConnection, eventID string) error {
	queue, err := h.queueClient.GetQueueByOwner(ctx, vcsConn.ID, "vcs_connections")
	if err != nil {
		return fmt.Errorf("unable to find queue for vcs connection: %w", err)
	}

	_, err = h.queueClient.EnqueueSignal(ctx, &queueclient.EnqueueSignalRequest{
		QueueID: queue.ID,
		Signal: &connectionEventSignal{
			VCSConnectionID: vcsConn.ID,
			VCSEventID:      eventID,
		},
	})
	if err != nil {
		return fmt.Errorf("unable to enqueue vcs connection event signal: %w", err)
	}

	return nil
}
