package scheduler_test

import (
	"testing"

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
