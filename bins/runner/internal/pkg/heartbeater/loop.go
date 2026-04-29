package heartbeater

import (
	"context"
	"time"

	"github.com/nuonco/nuon/sdks/nuon-runner-go/models"
	"go.uber.org/zap"

	"github.com/nuonco/nuon/bins/runner/internal/version"
	"github.com/nuonco/nuon/pkg/generics"
	"github.com/nuonco/nuon/pkg/metrics"
)

const (
	heartBeatErrBackoff time.Duration = time.Second * 5
)

func (h *HeartBeater) writeHeartBeat(ctx context.Context) error {
	tags := metrics.ToTags(
		map[string]string{
			"process": h.process,
		},
	)

	aliveDur := time.Since(h.startTS)
	req := &models.ServiceCreateRunnerHeartBeatRequest{
		AliveTime: generics.ToPtr(int64(aliveDur)),
		Version:   version.Version,
		Process:   h.process,
		ProcessID: h.processRegistrar.ProcessID(),
	}

	if _, err := h.apiClient.CreateHeartBeat(ctx, req); err != nil {
		return err
	}

	h.mw.Incr("heart_beat.incr", tags)
	h.mw.Timing("heart_beat.alive_time", aliveDur, tags)
	return nil
}

func (h *HeartBeater) loop(ctx context.Context) {
	ticker := time.NewTicker(h.settings.HeartBeatTimeout)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		h.l.Info("recording heart beat")

		// Bound each heartbeat write so a hung HTTP request (e.g. a stalled
		// HTTP/2 stream where http.DefaultTransport has no
		// ResponseHeaderTimeout) cannot park the loop forever and silently
		// stop heartbeats from reaching the API.
		writeCtx, cancel := context.WithTimeout(ctx, heartBeatErrBackoff)
		err := h.writeHeartBeat(writeCtx)
		cancel()
		if err != nil {
			h.l.Error("unable to write heart beat", zap.Error(err))
		}
	}
}
