package connectionevent

import (
	"fmt"

	"github.com/pkg/errors"
	"go.temporal.io/sdk/workflow"

	"github.com/nuonco/nuon/services/ctl-api/internal/app/apps/signals/v2/branches/vcspush"
	"github.com/nuonco/nuon/services/ctl-api/internal/app/vcs/worker/activities"
	"github.com/nuonco/nuon/services/ctl-api/internal/pkg/log"
	sharedactivities "github.com/nuonco/nuon/services/ctl-api/internal/pkg/workflows/activities"
)

func (s *Signal) Execute(ctx workflow.Context) error {
	l, err := log.WorkflowLogger(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to get logger")
	}

	// Fetch the VCS event
	event, err := activities.AwaitGetVCSEvent(ctx, activities.GetVCSEventRequest{
		VCSEventID: s.VCSEventID,
	})
	if err != nil {
		return errors.Wrap(err, "unable to get vcs event")
	}

	// Only process push events
	if event.EventType != "push" {
		l.Info(fmt.Sprintf("ignoring non-push event type: %s", event.EventType))
		return nil
	}

	// Parse the push payload
	pushInfo, err := parsePushEvent(event.Payload)
	if err != nil {
		l.Info(fmt.Sprintf("unable to parse push event payload: %v", err))
		return nil
	}

	l.Info(fmt.Sprintf("processing push event for repo=%s branch=%s", pushInfo.Repo, pushInfo.Branch))

	// Find matching app branches
	matches, err := activities.AwaitFindMatchingAppBranches(ctx, activities.FindMatchingAppBranchesRequest{
		VCSConnectionID: s.VCSConnectionID,
		Repo:            pushInfo.Repo,
		Branch:          pushInfo.Branch,
	})
	if err != nil {
		return errors.Wrap(err, "unable to find matching app branches")
	}

	if len(matches) == 0 {
		l.Info("no matching app branches found")
		return nil
	}

	l.Info(fmt.Sprintf("found %d matching app branches", len(matches)))

	// Fan out: send vcs-push signal to each matching app branch's queue
	for _, match := range matches {
		_, err := sharedactivities.AwaitEnqueueSignalToOwner(ctx, &sharedactivities.EnqueueSignalToOwnerRequest{
			OwnerID:   match.AppBranchID,
			OwnerType: "app_branches",
			Signal: &vcspush.Signal{
				AppBranchID:       match.AppBranchID,
				AppBranchConfigID: match.AppBranchConfigID,
			},
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to enqueue vcs-push signal for app branch %s: %v", match.AppBranchID, err))
			continue
		}

		l.Info(fmt.Sprintf("enqueued vcs-push signal for app branch %s", match.AppBranchID))
	}

	return nil
}
