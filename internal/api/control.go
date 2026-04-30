package api

import (
	"net/http"

	"github.com/cyrus/glutton/internal/scheduler"
	"github.com/labstack/echo/v4"
)

type controlHandlers struct {
	state *scheduler.State
	burst BurstController
}

func (h *controlHandlers) pause(c echo.Context) error {
	if h.state == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "scheduler not configured")
	}
	_ = h.state.Pause()
	return c.NoContent(http.StatusNoContent)
}

func (h *controlHandlers) resume(c echo.Context) error {
	if h.state == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "scheduler not configured")
	}
	_ = h.state.Resume()
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
	return c.NoContent(http.StatusNoContent)
}
