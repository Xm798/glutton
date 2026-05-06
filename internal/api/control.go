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
		h.bus.Publish(events.Event{Type: events.TypeServicePaused, Level: "info", Message: "paused by operator"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *controlHandlers) resume(c echo.Context) error {
	if h.state == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "scheduler not configured")
	}
	_ = h.state.Resume()
	if h.bus != nil {
		h.bus.Publish(events.Event{Type: events.TypeServiceResumed, Level: "info", Message: "resumed by operator"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *controlHandlers) burstNow(c echo.Context) error {
	if h.burst == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "burst not configured")
	}
	if h.state != nil && h.state.Get() == scheduler.QuotaReached {
		return echo.NewHTTPError(http.StatusConflict, "quota reached; reset before bursting")
	}
	var in struct {
		Minutes int   `json:"minutes"`
		Bytes   int64 `json:"bytes"`
	}
	_ = c.Bind(&in)
	if in.Minutes < 0 {
		in.Minutes = 0
	}
	if in.Bytes < 0 {
		in.Bytes = 0
	}
	if in.Minutes == 0 && in.Bytes == 0 {
		in.Minutes = 30 // default cap when caller specifies neither
	}
	h.burst.Burst(in.Minutes, in.Bytes)
	return c.NoContent(http.StatusNoContent)
}

func (h *controlHandlers) resetDaily(c echo.Context) error {
	if h.state == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "scheduler not configured")
	}
	_ = h.state.ResetQuota()
	if h.bus != nil {
		h.bus.Publish(events.Event{Type: events.TypeDailyResetManual, Level: "info", Message: "daily counter manually reset"})
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *controlHandlers) status(c echo.Context) error {
	if h.state == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "scheduler not configured")
	}
	st := h.state.Get()
	return c.JSON(http.StatusOK, map[string]any{
		"status":       st.String(),
		"burst_active": h.state.BurstActive(),
	})
}
