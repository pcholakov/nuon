package activities

import (
	"context"
	"fmt"

	"github.com/nuonco/nuon/services/ctl-api/internal/app"
)

type GetVCSEventRequest struct {
	VCSEventID string `validate:"required"`
}

// @temporal-gen-v2 activity
// @by-field VCSEventID
func (a *Activities) GetVCSEvent(ctx context.Context, req GetVCSEventRequest) (*app.VCSEvent, error) {
	var event app.VCSEvent
	res := a.db.WithContext(ctx).
		First(&event, "id = ?", req.VCSEventID)
	if res.Error != nil {
		return nil, fmt.Errorf("unable to get vcs event: %w", res.Error)
	}
	return &event, nil
}
