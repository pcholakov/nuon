package connectionevent

import (
	"github.com/pkg/errors"
	"go.temporal.io/sdk/workflow"

	"github.com/nuonco/nuon/services/ctl-api/internal/app/vcs/worker/activities"
	"github.com/nuonco/nuon/services/ctl-api/internal/pkg/queue/signal"
)

const SignalType signal.SignalType = "vcs_connection_event"

type Signal struct {
	VCSConnectionID string `json:"vcs_connection_id"`
	VCSEventID      string `json:"vcs_event_id"`
}

var _ signal.Signal = (*Signal)(nil)

func (s *Signal) Type() signal.SignalType {
	return SignalType
}

func (s *Signal) Validate(ctx workflow.Context) error {
	if s.VCSConnectionID == "" {
		return errors.New("vcs_connection_id is required")
	}
	if s.VCSEventID == "" {
		return errors.New("vcs_event_id is required")
	}

	_, err := activities.AwaitGetVCSConnection(ctx, activities.GetVCSConnectionRequest{
		VCSConnectionID: s.VCSConnectionID,
	})
	if err != nil {
		return errors.Wrap(err, "vcs connection not found")
	}

	return nil
}
