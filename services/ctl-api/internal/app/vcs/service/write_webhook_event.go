package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/nuonco/nuon/services/ctl-api/internal/app"
)

// verifyGitHubSignature validates the X-Hub-Signature-256 header against the raw request body.
func verifyGitHubSignature(secret string, signature string, body []byte) bool {
	if signature == "" || secret == "" {
		return false
	}

	// GitHub sends "sha256=<hex>"
	sig := strings.TrimPrefix(signature, "sha256=")
	if sig == signature {
		// No "sha256=" prefix — invalid format
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(sig), []byte(expected))
}

// @ID						WriteWebhookEvent
// @Summary					Write a VCS webhook event (shared per subscription)
// @Description				Receives webhook events for a webhook subscription and fans out to all VCS connections sharing that GitHub installation
// @Param					subscription_id	path	string	true	"Webhook Subscription ID"
// @Tags					vcs
// @Accept					json
// @Produce					json
// @Failure					400	{object}	stderr.ErrResponse
// @Failure					401	{object}	stderr.ErrResponse
// @Failure					404	{object}	stderr.ErrResponse
// @Failure					500	{object}	stderr.ErrResponse
// @Success					200	{object}	[]app.VCSEvent
// @Router					/v1/vcs/webhooks/{subscription_id}/events [post]
func (s *service) WriteWebhookEvent(ctx *gin.Context) {
	subscriptionID := ctx.Param("subscription_id")

	// Look up the webhook subscription.
	var sub app.VCSWebhookSubscription
	if err := s.db.WithContext(ctx).First(&sub, "id = ?", subscriptionID).Error; err != nil {
		ctx.Error(fmt.Errorf("webhook subscription not found: %w", err))
		return
	}

	// Read the raw body for signature verification.
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.Error(fmt.Errorf("unable to read request body: %w", err))
		return
	}

	// Verify the GitHub webhook signature.
	signature := ctx.GetHeader("X-Hub-Signature-256")
	if !verifyGitHubSignature(sub.WebhookSecret, signature, body) {
		s.l.Warn("webhook signature verification failed",
			zap.String("subscription_id", subscriptionID),
		)
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	// Parse the payload from the raw body (can't use ShouldBindJSON since body is already consumed).
	var payload app.VCSEventPayload
	if err := parsePayload(body, &payload); err != nil {
		ctx.Error(fmt.Errorf("unable to parse event payload: %w", err))
		return
	}

	// Extract event type from GitHub header.
	eventType := ctx.GetHeader("X-GitHub-Event")
	if eventType == "" {
		eventType = "unknown"
	}

	// Find ALL VCS connections for this GitHub installation (across orgs).
	var vcsConns []app.VCSConnection
	if err := s.db.WithContext(ctx).
		Where("github_install_id = ?", sub.GithubInstallID).
		Find(&vcsConns).Error; err != nil {
		ctx.Error(fmt.Errorf("unable to find vcs connections: %w", err))
		return
	}

	if len(vcsConns) == 0 {
		s.l.Warn("no vcs connections found for webhook subscription",
			zap.String("subscription_id", subscriptionID),
			zap.String("github_install_id", sub.GithubInstallID),
		)
		ctx.JSON(http.StatusOK, []app.VCSEvent{})
		return
	}

	// Fan out: create a VCSEvent and enqueue signal for each VCS connection.
	var events []app.VCSEvent
	for _, vcsConn := range vcsConns {
		event := app.VCSEvent{
			OrgID:           vcsConn.OrgID,
			VCSConnectionID: vcsConn.ID,
			EventType:       eventType,
			Payload:         payload,
			Status: &app.CompositeStatus{
				CreatedAtTS:            time.Now().Unix(),
				Status:                 app.StatusSuccess,
				StatusHumanDescription: fmt.Sprintf("received %s event", eventType),
			},
		}

		if err := s.db.WithContext(ctx).Create(&event).Error; err != nil {
			s.l.Error("unable to store vcs event",
				zap.String("vcs_connection_id", vcsConn.ID),
				zap.Error(err),
			)
			continue
		}

		// Enqueue signal to process this event (non-blocking)
		if err := s.helpers.EnqueueVCSConnectionEvent(ctx, &vcsConn, event.ID); err != nil {
			s.l.Warn("failed to enqueue vcs connection event signal",
				zap.String("vcs_connection_id", vcsConn.ID),
				zap.String("event_id", event.ID),
				zap.Error(err),
			)
		}

		events = append(events, event)
	}

	ctx.JSON(http.StatusOK, events)
}

func parsePayload(body []byte, payload *app.VCSEventPayload) error {
	return json.Unmarshal(body, payload)
}
