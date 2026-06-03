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

type seriesPoint struct {
	T     int64 `json:"t"`
	Bytes int64 `json:"bytes"`
}

// series serves GET /api/stats/series?range=1h|1d|1w|1m as a dense, gap-filled,
// server-downsampled time series. 1h/1d read minute_samples; 1w/1m read the
// hourly traffic_buckets (summed across sources). Unknown range defaults to 1d.
func (h *statsHandlers) series(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "store not configured")
	}

	var (
		dur     time.Duration
		step    int64 // output bucket size, seconds
		useHour bool
	)
	switch c.QueryParam("range") {
	case "1h":
		dur, step, useHour = time.Hour, 60, false
	case "1w":
		dur, step, useHour = 7*24*time.Hour, 3600, true
	case "1m":
		dur, step, useHour = 30*24*time.Hour, 4*3600, true
	default: // "1d" and anything unrecognized
		dur, step, useHour = 24*time.Hour, 600, false
	}

	now := time.Now()
	cutoff := now.Add(-dur).Unix() / step * step // align down to step boundary
	end := now.Unix()

	sums := make(map[int64]int64)
	add := func(bucket, bytes int64) { sums[bucket/step*step] += bytes }
	if useHour {
		rows, err := store.TrafficSinceBucket(h.store, cutoff)
		if err != nil {
			return err
		}
		for _, r := range rows {
			add(r.HourBucket, r.Bytes)
		}
	} else {
		rows, err := store.MinuteSamplesSince(h.store, cutoff)
		if err != nil {
			return err
		}
		for _, r := range rows {
			add(r.MinuteBucket, r.Bytes)
		}
	}

	points := make([]seriesPoint, 0, (end-cutoff)/step+1)
	for ts := cutoff; ts <= end; ts += step {
		points = append(points, seriesPoint{T: ts, Bytes: sums[ts]})
	}
	return c.JSON(http.StatusOK, map[string]any{"step": step, "points": points})
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
