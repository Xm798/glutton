package scheduler_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/scheduler"
	"github.com/stretchr/testify/require"
)

func TestStateTransitions(t *testing.T) {
	s := scheduler.NewState()
	require.Equal(t, scheduler.Idle, s.Get())

	require.NoError(t, s.Activate())
	require.Equal(t, scheduler.Running, s.Get())

	require.NoError(t, s.Pause())
	require.Equal(t, scheduler.Paused, s.Get())

	require.NoError(t, s.Resume())
	require.Equal(t, scheduler.Running, s.Get())

	require.NoError(t, s.QuotaReached())
	require.Equal(t, scheduler.QuotaReached, s.Get())

	require.NoError(t, s.ResetQuota())
	require.Equal(t, scheduler.Idle, s.Get())
}

func TestPauseFromIdleIsNoOp(t *testing.T) {
	s := scheduler.NewState()
	require.NoError(t, s.Pause())
	require.Equal(t, scheduler.Paused, s.Get())
}

func TestResumeFromPausedReturnsToIdleWhenInactive(t *testing.T) {
	s := scheduler.NewState()
	_ = s.Pause()
	require.NoError(t, s.Resume())
	require.Equal(t, scheduler.Idle, s.Get())
}

func TestBurstKeepsRunningAcrossDeactivate(t *testing.T) {
	s := scheduler.NewState()
	now := time.Unix(1_700_000_000, 0)
	s.SetNow(func() time.Time { return now })

	// Activate with a 30s burst deadline.
	require.NoError(t, s.Activate(now.Add(30*time.Second)))
	require.Equal(t, scheduler.Running, s.Get())
	require.True(t, s.BurstActive())

	// Deactivate is a no-op while burst is active.
	require.NoError(t, s.Deactivate())
	require.Equal(t, scheduler.Running, s.Get())

	// Once the deadline passes, Deactivate works again.
	now = now.Add(time.Minute)
	require.False(t, s.BurstActive())
	require.NoError(t, s.Deactivate())
	require.Equal(t, scheduler.Idle, s.Get())
}

func TestEndBurstClearsDeadline(t *testing.T) {
	s := scheduler.NewState()
	now := time.Unix(1_700_000_000, 0)
	s.SetNow(func() time.Time { return now })

	require.NoError(t, s.Activate(now.Add(time.Hour)))
	require.True(t, s.BurstActive())

	s.EndBurst()
	require.False(t, s.BurstActive())
	require.NoError(t, s.Deactivate())
	require.Equal(t, scheduler.Idle, s.Get())
}

func TestActivateWithLongerBurstReplacesShorter(t *testing.T) {
	s := scheduler.NewState()
	now := time.Unix(1_700_000_000, 0)
	s.SetNow(func() time.Time { return now })

	require.NoError(t, s.Activate(now.Add(10*time.Second)))
	require.NoError(t, s.Activate(now.Add(time.Minute))) // longer deadline wins

	now = now.Add(15 * time.Second)
	require.True(t, s.BurstActive(), "longer deadline should still be active")
}

func TestTransitionHookFires(t *testing.T) {
	s := scheduler.NewState()
	var fromN, toN atomic.Int32
	s.SetTransitionHook(func(from, to scheduler.Status) {
		fromN.Add(1)
		toN.Add(1)
		_ = from
		_ = to
	})
	require.NoError(t, s.Activate())  // Idle → Running
	require.NoError(t, s.Pause())     // Running → Paused
	require.NoError(t, s.Activate())  // no observable change
	require.NoError(t, s.Resume())    // Paused → Running
	require.Equal(t, int32(3), toN.Load())
}

func TestBurstActiveDoesNotOverrideQuotaReached(t *testing.T) {
	s := scheduler.NewState()
	now := time.Unix(1_700_000_000, 0)
	s.SetNow(func() time.Time { return now })
	require.NoError(t, s.QuotaReached())
	require.NoError(t, s.Activate(now.Add(time.Minute)))
	require.Equal(t, scheduler.QuotaReached, s.Get())
}
