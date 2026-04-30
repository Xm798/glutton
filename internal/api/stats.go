package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/cyrus/glutton/internal/store"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type statsHandlers struct {
	store   *gorm.DB
	counter *LiveCounters
	loc     *time.Location
}

// resolveSince treats a missing or zero `since` as "today midnight in the
// configured TZ" so per-source totals align with the Today KPI.
func (h *statsHandlers) resolveSince(c echo.Context) int64 {
	if since, _ := strconv.ParseInt(c.QueryParam("since"), 10, 64); since > 0 {
		return since
	}
	loc := h.loc
	if loc == nil {
		loc = time.Local
	}
	return store.DayStart(time.Now(), loc)
}

func (h *statsHandlers) live(c echo.Context) error {
	rate, today, month, updated := int64(0), int64(0), int64(0), int64(0)
	if h.counter != nil {
		r, t, m, u := h.counter.Snapshot()
		rate, today, month = r, t, m
		updated = u.Unix()
	}
	return c.JSON(http.StatusOK, map[string]any{
		"current_rate_bps": rate,
		"today_bytes":      today,
		"month_bytes":      month,
		"updated_at":       updated,
	})
}

func (h *statsHandlers) history(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "store not configured")
	}
	since, _ := strconv.ParseInt(c.QueryParam("since"), 10, 64)
	rows, err := store.TrafficSinceBucket(h.store, since)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, rows)
}

func (h *statsHandlers) bySource(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "store not configured")
	}
	rows, err := store.TrafficBySource(h.store, h.resolveSince(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, rows)
}
