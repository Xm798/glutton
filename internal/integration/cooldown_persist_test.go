package integration_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/api"
	"github.com/cyrus/glutton/internal/events"
	"github.com/cyrus/glutton/internal/notify"
	"github.com/cyrus/glutton/internal/sources"
	"github.com/cyrus/glutton/internal/store"
	"github.com/stretchr/testify/require"
)

// TestH4SourceCooldownPersistedToHistory mimics the production wiring: bus +
// notifier with PersistEvent. After a cooldown is computed and a
// source_cooldown event is published, the persisted history row must be
// retrievable via /api/events/history and carry a non-zero EventID.
func TestH4SourceCooldownPersistedToHistory(t *testing.T) {
	db, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close(db) })

	bus := events.NewBus(8)
	t.Cleanup(bus.Close)

	notifier := notify.New(notify.Config{
		SubscribedTypes: []string{events.TypeSourceCooldown},
		PersistEvent: func(_ context.Context, e events.Event) error {
			return store.InsertEvent(db, &store.Event{
				EventID: e.ID, Ts: e.TS.Unix(), Level: e.Level, Type: e.Type, Message: e.Message,
			})
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go notifier.Run(ctx, bus)
	<-notifier.Ready()

	const sourceID uint = 7
	cd := sources.CooldownFor(3)
	require.Equal(t, 4*time.Minute, cd, "sanity: cooldown formula unchanged")
	bus.Publish(events.Event{
		Type:    events.TypeSourceCooldown,
		Level:   "warn",
		Message: "source 7 cooled down for 4m0s after 3 consecutive failures",
		Data: map[string]any{
			"source_id":            sourceID,
			"cooldown_seconds":     int64(cd.Seconds()),
			"consecutive_failures": 3,
		},
	})

	require.Eventually(t, func() bool {
		rows, err := store.ListEvents(db, 0, 10)
		if err != nil {
			return false
		}
		for _, r := range rows {
			if r.Type == events.TypeSourceCooldown && r.EventID > 0 {
				return true
			}
		}
		return false
	}, 2*time.Second, 25*time.Millisecond, "source_cooldown event should be persisted with non-zero EventID")

	srv := api.New(api.Deps{Store: db})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/events/history?types=source_cooldown", nil)
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	require.Contains(t, body, `"Type":"source_cooldown"`)
	require.Contains(t, body, `"id":`)

	rows, err := store.ListEvents(db, 0, 10)
	require.NoError(t, err)
	var got *store.Event
	for i := range rows {
		if rows[i].Type == events.TypeSourceCooldown {
			got = &rows[i]
			break
		}
	}
	require.NotNil(t, got)
	require.NotZero(t, got.EventID)
}

// TestH4DefaultSubscribedEventsIncludesCooldown is a guard against silent
// regressions of the operator-notification policy: source_cooldown should be
// in the default SubscribedEvents list emitted by loadRuntime so a fresh
// install also forwards cooldowns to shoutrrr destinations.
func TestH4DefaultSubscribedEventsIncludesCooldown(t *testing.T) {
	require.Contains(t, events.DefaultSubscribedTypes(), events.TypeSourceCooldown)
}
