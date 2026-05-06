package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/cyrus/glutton/internal/store"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type eventsHandlers struct {
	store *gorm.DB
}

const (
	defaultEventsLimit = 100
	maxEventsLimit     = 500
)

// list serves GET /api/events/history?since=<unix>&limit=<n>&types=a,b,c
// Returns events ordered by ts DESC. types filter is pushed into SQL so
// `limit` applies post-filter (M-3): a request with a tight types filter
// gets up to `limit` matching rows, not "matches among the most-recent
// limit rows".
func (h *eventsHandlers) list(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "store not configured")
	}
	since, _ := strconv.ParseInt(c.QueryParam("since"), 10, 64)
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = defaultEventsLimit
	}
	if limit > maxEventsLimit {
		limit = maxEventsLimit
	}

	var types []string
	if raw := c.QueryParam("types"); raw != "" {
		for _, t := range strings.Split(raw, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				types = append(types, t)
			}
		}
	}
	rows, err := store.ListEventsFiltered(h.store, since, limit, types)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, rows)
}
