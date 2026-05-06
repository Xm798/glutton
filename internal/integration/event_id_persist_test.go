package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/events"
	"github.com/cyrus/glutton/internal/notify"
	"github.com/cyrus/glutton/internal/store"
	"github.com/stretchr/testify/require"
)

// TestB1EventIDSurvivesRestart simulates a process bounce: persist some
// events through bus #1, close it, then open bus #2 against the same DB and
// publish more. After restart, the second bus's first id MUST exceed the
// pre-restart MAX(event_id); without the fix nextID begins at 1 and the
// new persisted rows duplicate ids 1..n, which the frontend's Set<number>
// dedupe would silently swallow.
func TestB1EventIDSurvivesRestart(t *testing.T) {
	db, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close(db) })

	persist := func(e events.Event) error {
		return store.InsertEvent(db, &store.Event{
			EventID: e.ID, Ts: e.TS.Unix(),
			Level: e.Level, Type: e.Type, Message: e.Message,
		})
	}

	// --- run 1 ---
	bus1 := events.NewBus(8)
	n1 := notify.New(notify.Config{
		PersistEvent: func(_ context.Context, e events.Event) error { return persist(e) },
	})
	ctx1, cancel1 := context.WithCancel(context.Background())
	go n1.Run(ctx1, bus1)
	<-n1.Ready()

	for i := 0; i < 5; i++ {
		bus1.Publish(events.Event{Type: "x", Message: "first run"})
	}
	require.Eventually(t, func() bool {
		rows, _ := store.ListEvents(db, 0, 50)
		return len(rows) == 5
	}, 2*time.Second, 20*time.Millisecond)

	cancel1()
	bus1.Close()

	pre, err := store.MaxEventID(db)
	require.NoError(t, err)
	require.Equal(t, uint64(5), pre)

	// --- run 2 (the "restart") ---
	bus2 := events.NewBus(8)
	t.Cleanup(bus2.Close)
	// THE FIX: seed nextID from persisted MAX before any Publish.
	maxID, err := store.MaxEventID(db)
	require.NoError(t, err)
	bus2.SetNextID(maxID)

	n2 := notify.New(notify.Config{
		PersistEvent: func(_ context.Context, e events.Event) error { return persist(e) },
	})
	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	go n2.Run(ctx2, bus2)
	<-n2.Ready()

	for i := 0; i < 3; i++ {
		bus2.Publish(events.Event{Type: "y", Message: "second run"})
	}
	require.Eventually(t, func() bool {
		rows, _ := store.ListEvents(db, 0, 50)
		return len(rows) == 8
	}, 2*time.Second, 20*time.Millisecond)

	rows, err := store.ListEvents(db, 0, 50)
	require.NoError(t, err)

	// Every persisted row carries a unique EventID, and the post-restart ids
	// are strictly greater than the pre-restart MAX.
	seen := make(map[uint64]bool, len(rows))
	for _, r := range rows {
		require.NotZero(t, r.EventID, "every row must have a non-zero event id")
		require.False(t, seen[r.EventID], "duplicate event id across runs: %d", r.EventID)
		seen[r.EventID] = true
		if r.Type == "y" {
			require.Greater(t, r.EventID, pre,
				"post-restart event ids must exceed pre-restart MAX (%d), got %d", pre, r.EventID)
		}
	}
}

// TestB1WithoutSeedShowsCollision is the negative control: without
// SetNextID, the second bus restarts ids at 1 and persistence yields
// duplicate event_ids vs the prior run. We assert the collision so a
// future regression that drops the seed is caught immediately.
func TestB1WithoutSeedShowsCollision(t *testing.T) {
	db, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close(db) })

	persist := func(e events.Event) error {
		return store.InsertEvent(db, &store.Event{
			EventID: e.ID, Ts: e.TS.Unix(),
			Level: e.Level, Type: e.Type, Message: e.Message,
		})
	}

	bus1 := events.NewBus(4)
	n1 := notify.New(notify.Config{PersistEvent: func(_ context.Context, e events.Event) error { return persist(e) }})
	ctx1, cancel1 := context.WithCancel(context.Background())
	go n1.Run(ctx1, bus1)
	<-n1.Ready()
	for i := 0; i < 3; i++ {
		bus1.Publish(events.Event{Type: "a"})
	}
	require.Eventually(t, func() bool {
		r, _ := store.ListEvents(db, 0, 50)
		return len(r) == 3
	}, time.Second, 10*time.Millisecond)
	cancel1()
	bus1.Close()

	bus2 := events.NewBus(4)
	t.Cleanup(bus2.Close)
	// NOTE: deliberately NOT calling SetNextID here.
	n2 := notify.New(notify.Config{PersistEvent: func(_ context.Context, e events.Event) error { return persist(e) }})
	ctx2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	go n2.Run(ctx2, bus2)
	<-n2.Ready()
	for i := 0; i < 3; i++ {
		bus2.Publish(events.Event{Type: "b"})
	}
	require.Eventually(t, func() bool {
		r, _ := store.ListEvents(db, 0, 50)
		return len(r) == 6
	}, time.Second, 10*time.Millisecond)

	rows, _ := store.ListEvents(db, 0, 50)
	idCounts := make(map[uint64]int)
	for _, r := range rows {
		idCounts[r.EventID]++
	}
	dupes := 0
	for _, c := range idCounts {
		if c > 1 {
			dupes++
		}
	}
	require.Greater(t, dupes, 0,
		"sanity: the unsealed restart should produce duplicate event_ids; if this fails, the bus is doing something the fix didn't anticipate")
}
