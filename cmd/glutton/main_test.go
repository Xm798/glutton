package main

import (
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/events"
	"github.com/cyrus/glutton/internal/scheduler"
	"github.com/stretchr/testify/require"
)

func TestStringSetsEqualOrderInsensitive(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{nil, nil, true},
		{[]string{}, nil, true},
		{[]string{"a", "b"}, []string{"b", "a"}, true},
		{[]string{"a", "b", "a"}, []string{"a", "b"}, true}, // dedup-collapsed
		{[]string{"a", "b"}, []string{"a", "c"}, false},
		{[]string{"a"}, []string{"a", "b"}, false},
		{[]string{"x", "y", "z"}, []string{"z", "y", "x"}, true},
	}
	for _, tc := range cases {
		require.Equalf(t, tc.want, stringSetsEqual(tc.a, tc.b), "a=%v b=%v", tc.a, tc.b)
	}
}

func TestDedupSorted(t *testing.T) {
	require.Equal(t, []string{"a"}, dedupSorted([]string{"a", "a"}))
	require.Equal(t, []string{"a", "b", "c"}, dedupSorted([]string{"a", "a", "b", "c", "c"}))
	require.Equal(t, []string{}, dedupSorted([]string{}))
	require.Equal(t, []string{"a"}, dedupSorted([]string{"a"}))
}

func TestDrainConfigUpdatesSwallowsBufferedEvents(t *testing.T) {
	bus := events.NewBus(8)
	defer bus.Close()
	ch := bus.SubscribeBuffered(8)

	for i := 0; i < 4; i++ {
		bus.Publish(events.Event{Type: events.TypeConfigUpdated})
	}
	// First receive, then drain.
	<-ch
	drainConfigUpdates(ch)
	require.Equal(t, 0, len(ch), "channel should be drained")

	// Non-config event types are also drained (defensive, see helper comment).
	bus.Publish(events.Event{Type: events.TypeServiceStarted})
	bus.Publish(events.Event{Type: events.TypeConfigUpdated})
	drainConfigUpdates(ch)
	require.Equal(t, 0, len(ch))
}

// TestByteOnlyBurstUsesSafetyTimeout verifies that when Burst is invoked with
// no minutes (bytes-only, never reached because nobody is downloading) the
// State eventually flips back to non-burst after the safety timeout, and not
// 24h later.
func TestByteOnlyBurstUsesSafetyTimeout(t *testing.T) {
	// Sanity: the constant is fixed at 2h.
	require.Equal(t, 2*time.Hour, byteOnlyBurstSafetyTimeout)

	// Drive the State directly with a clock so we can fast-forward past the
	// safety timeout without sleeping. This bypasses burstImpl's real-time
	// timer (uncommonly long) but exercises the same deadline contract:
	// burstImpl.Burst computes `time.Now() + byteOnlyBurstSafetyTimeout` and
	// hands it to State.Activate. Once the clock advances past that moment,
	// BurstActive() must report false and Deactivate() must drop Running.
	st := scheduler.NewState()
	now := time.Unix(1_700_000_000, 0)
	st.SetNow(func() time.Time { return now })

	deadline := now.Add(byteOnlyBurstSafetyTimeout)
	require.NoError(t, st.Activate(deadline))
	require.Equal(t, scheduler.Running, st.Get())
	require.True(t, st.BurstActive())

	// Just before the deadline, still active.
	now = deadline.Add(-time.Second)
	require.True(t, st.BurstActive())

	// At/after the deadline, Burst is over and Deactivate works.
	now = deadline.Add(time.Second)
	require.False(t, st.BurstActive())
	require.NoError(t, st.Deactivate())
	require.Equal(t, scheduler.Idle, st.Get())
}

func TestBurstImplDeadlineByteOnlyMatchesSafetyTimeout(t *testing.T) {
	st := scheduler.NewState()
	bus := events.NewBus(8)
	defer bus.Close()
	b := &burstImpl{state: st, bytesUsed: func() int64 { return 0 }, bus: bus}

	before := time.Now()
	// 0 minutes + 1<<30 bytes that nobody will ever drain.
	b.Burst(0, 1<<30)
	after := time.Now()

	// State should be Running, BurstActive true, and the burst deadline
	// should be within a few ms of (now + safety_timeout). We can't read the
	// deadline directly, but we can probe BurstActive at the boundary.
	require.True(t, st.BurstActive())

	// Advance State's view of time to just before safety_timeout: still active.
	st.SetNow(func() time.Time { return before.Add(byteOnlyBurstSafetyTimeout - time.Second) })
	require.True(t, st.BurstActive())

	// Advance past safety_timeout: inactive (deadline crossed).
	st.SetNow(func() time.Time { return after.Add(byteOnlyBurstSafetyTimeout + time.Second) })
	require.False(t, st.BurstActive())
}

// TestM5TzdataEmbedded asserts that the binary can resolve common non-UTC
// timezones without relying on the host's /usr/share/zoneinfo. This protects
// distroless/static deployments from booting and immediately crashing on a
// LoadLocation error when TZ=Asia/Shanghai or similar is set.
func TestM5TzdataEmbedded(t *testing.T) {
	for _, name := range []string{
		"Asia/Shanghai", "Europe/London", "America/New_York", "UTC",
	} {
		loc, err := time.LoadLocation(name)
		require.NoError(t, err, "LoadLocation(%q) failed; tzdata not embedded?", name)
		require.NotNil(t, loc)
		require.Equal(t, name, loc.String())
	}
}
