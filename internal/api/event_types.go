package api

import (
	"net/http"

	"github.com/cyrus/glutton/internal/events"
	"github.com/labstack/echo/v4"
)

// allKnownEventTypes is the canonical inventory the UI uses to populate
// "subscribable event types" pickers / settings forms. Centralising it here
// ensures the backend (notifier defaults) and frontend (settings) never
// drift again — the FE pulls this list at runtime instead of hardcoding it.
//
// Order is operator-meaningful (most-actionable first) so the UI can render
// it as-is without re-sorting.
var allKnownEventTypes = []string{
	events.TypeQuotaReachedDaily,
	events.TypeQuotaReachedMonth,
	events.TypeSourcesMassFailure,
	events.TypeSourceCooldown,
	events.TypeSourceError,
	events.TypeBurstStarted,
	events.TypeBurstEnded,
	events.TypeServiceStarted,
	events.TypeServiceStopped,
	events.TypeServicePaused,
	events.TypeServiceResumed,
	events.TypeStateChanged,
	events.TypeDailyReset,
	events.TypeDailyResetManual,
	events.TypeMonthlyReset,
	events.TypeConfigUpdated,
	events.TypeSourceCreated,
	events.TypeSourceUpdated,
	events.TypeSourceDeleted,
}

type eventTypesResponse struct {
	All     []string `json:"all"`
	Default []string `json:"default"`
}

// EventTypesHandler returns the full list of valid event types plus the set
// subscribed by default on a fresh install. The frontend should treat the
// `default` list as authoritative when no `subscribed_events` config row
// exists yet.
func EventTypesHandler() echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.JSON(http.StatusOK, eventTypesResponse{
			All:     append([]string(nil), allKnownEventTypes...),
			Default: events.DefaultSubscribedTypes(),
		})
	}
}
