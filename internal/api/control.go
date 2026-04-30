package api

import (
	"net/http"

	"github.com/cyrus/glutton/internal/events"
	"github.com/cyrus/glutton/internal/scheduler"
	"github.com/labstack/echo/v4"
)

type controlHandlers struct {
	state *scheduler.State
	burst BurstController
	bus   *events.Bus
}

func (h *controlHandlers) pause(c echo.Context) error {
	if h.state == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "scheduler not configured")
	}
	_ = h.state.Pause()
	if h.bus != nil {
		h.bus.Publish(events.Event{Type: "service_paused", Level: "info", Message: "paused by operator"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *controlHandlers) resume(c echo.Context) error {
	if h.state == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "scheduler not configured")
	}
	_ = h.state.Resume()
	if h.bus != nil {
		h.bus.Publish(events.Event{Type: "service_resumed", Level: "info", Message: "resumed by operator"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *controlHandlers) burstNow(c echo.Context) error {
	if h.burst == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "burst not configured")
	}
	var in struct {
		Minutes int `json:"minutes"`
	}
	_ = c.Bind(&in)
	if in.Minutes <= 0 {
		in.Minutes = 30
	}
	h.burst.Burst(in.Minutes)
	return c.NoContent(http.StatusNoContent)
}

func (h *controlHandlers) resetDaily(c echo.Context) error {
	if h.state == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "scheduler not configured")
	}
	_ = h.state.ResetQuota()
	if h.bus != nil {
		h.bus.Publish(events.Event{Type: "daily_reset_manual", Level: "info", Message: "daily counter manually reset"})
	}
	return c.NoContent(http.StatusNoContent)
}
