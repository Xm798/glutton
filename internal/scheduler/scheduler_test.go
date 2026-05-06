package scheduler_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/scheduler"
	"github.com/stretchr/testify/require"
)

func TestInWindow(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	w, err := scheduler.ParseWindows([]string{"* 0-6 * * *"}, loc)
	require.NoError(t, err)

	// 03:30 UTC — should be in window.
	t1 := time.Date(2026, 4, 30, 3, 30, 0, 0, loc)
	require.True(t, w.Contains(t1))
	// 12:00 UTC — outside window.
	t2 := time.Date(2026, 4, 30, 12, 0, 0, 0, loc)
	require.False(t, w.Contains(t2))
}

func TestSchedulerActivatesInWindowDeactivatesOutside(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	w, err := scheduler.ParseWindows([]string{"* 0-6 * * *"}, loc)
	require.NoError(t, err)

	clock := newMockClock(time.Date(2026, 4, 30, 3, 0, 0, 0, loc))
	st := scheduler.NewState()
	s := scheduler.New(scheduler.Config{
		State:            st,
		Windows:          w,
		DailyQuotaBytes:  0, // unlimited
		Now:              clock.Now,
		BytesUsedDaily:   func() int64 { return 0 },
		BytesUsedMonthly: func() int64 { return 0 },
	})

	// In window.
	s.Tick()
	require.Equal(t, scheduler.Running, st.Get())

	// Move to 12:00.
	clock.Set(time.Date(2026, 4, 30, 12, 0, 0, 0, loc))
	s.Tick()
	require.Equal(t, scheduler.Idle, st.Get())
}

func TestSchedulerEntersQuotaReached(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	w, err := scheduler.ParseWindows([]string{"* * * * *"}, loc)
	require.NoError(t, err)

	clock := newMockClock(time.Date(2026, 4, 30, 3, 0, 0, 0, loc))
	st := scheduler.NewState()
	used := int64(150 * 1024 * 1024 * 1024) // 150 GB used
	s := scheduler.New(scheduler.Config{
		State:            st,
		Windows:          w,
		DailyQuotaBytes:  100 * 1024 * 1024 * 1024,
		Now:              clock.Now,
		BytesUsedDaily:   func() int64 { return used },
		BytesUsedMonthly: func() int64 { return 0 },
	})

	s.Tick()
	require.Equal(t, scheduler.QuotaReached, st.Get())
}

func TestRunLoopExitsOnContext(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	w, _ := scheduler.ParseWindows([]string{"* * * * *"}, loc)
	st := scheduler.NewState()
	clock := newMockClock(time.Now().UTC())
	s := scheduler.New(scheduler.Config{
		State: st, Windows: w,
		Now: clock.Now, TickInterval: 10 * time.Millisecond,
		BytesUsedDaily:   func() int64 { return 0 },
		BytesUsedMonthly: func() int64 { return 0 },
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	s.Run(ctx) // returns when ctx done
}

// --- helpers ---

type mockClock struct {
	mu sync.Mutex
	t  time.Time
}

func newMockClock(t time.Time) *mockClock { return &mockClock{t: t} }
func (m *mockClock) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.t
}
func (m *mockClock) Set(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.t = t
}

func TestSetWindowsHotSwap(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	wInside, err := scheduler.ParseWindows([]string{"* * * * *"}, loc)
	require.NoError(t, err)
	wOutside, err := scheduler.ParseWindows([]string{"0 0 1 1 *"}, loc) // Jan 1 00:00 only
	require.NoError(t, err)

	clock := newMockClock(time.Date(2026, 4, 30, 3, 0, 0, 0, loc))
	st := scheduler.NewState()
	s := scheduler.New(scheduler.Config{
		State: st, Windows: wInside,
		Now:              clock.Now,
		BytesUsedDaily:   func() int64 { return 0 },
		BytesUsedMonthly: func() int64 { return 0 },
	})

	s.Tick()
	require.Equal(t, scheduler.Running, st.Get())

	s.SetWindows(wOutside)
	s.Tick()
	require.Equal(t, scheduler.Idle, st.Get())
}

func TestSetQuotasHotSwap(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	w, err := scheduler.ParseWindows([]string{"* * * * *"}, loc)
	require.NoError(t, err)

	clock := newMockClock(time.Date(2026, 4, 30, 3, 0, 0, 0, loc))
	st := scheduler.NewState()
	used := int64(50 * 1024 * 1024 * 1024) // 50 GB
	s := scheduler.New(scheduler.Config{
		State: st, Windows: w,
		DailyQuotaBytes:  100 * 1024 * 1024 * 1024,
		Now:              clock.Now,
		BytesUsedDaily:   func() int64 { return used },
		BytesUsedMonthly: func() int64 { return 0 },
	})
	s.Tick()
	require.Equal(t, scheduler.Running, st.Get())

	// Lower the cap below current usage → quota_reached on next tick.
	s.SetQuotas(10*1024*1024*1024, 0)
	s.Tick()
	require.Equal(t, scheduler.QuotaReached, st.Get())

	// Raise the cap → ResetQuota gets us back to Idle, next tick goes Running.
	s.SetQuotas(0, 0)
	require.NoError(t, st.ResetQuota())
	s.Tick()
	require.Equal(t, scheduler.Running, st.Get())
}

func TestTickRespectsBurstActive(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	// A window that never matches our clock (Feb 31 doesn't exist; use Jan 1 00:00 only).
	w, err := scheduler.ParseWindows([]string{"0 0 1 1 *"}, loc)
	require.NoError(t, err)

	clock := newMockClock(time.Date(2026, 4, 30, 3, 0, 0, 0, loc))
	st := scheduler.NewState()
	st.SetNow(clock.Now)
	s := scheduler.New(scheduler.Config{
		State: st, Windows: w,
		Now:              clock.Now,
		BytesUsedDaily:   func() int64 { return 0 },
		BytesUsedMonthly: func() int64 { return 0 },
	})

	// Burst override: outside window but burst active.
	require.NoError(t, st.Activate(clock.Now().Add(time.Minute)))
	require.Equal(t, scheduler.Running, st.Get())
	s.Tick()
	require.Equal(t, scheduler.Running, st.Get(), "burst should hold Running across Tick's Deactivate")

	// After burst expires, Tick deactivates.
	clock.Set(clock.Now().Add(2 * time.Minute))
	s.Tick()
	require.Equal(t, scheduler.Idle, st.Get())
}
