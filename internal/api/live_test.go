package api_test

import (
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/api"
	"github.com/stretchr/testify/require"
)

func TestLiveCountersUpdatedAtAdvancesEvenWhenValuesUnchanged(t *testing.T) {
	l := &api.LiveCounters{}
	l.Set(0, 0, 0)
	_, _, _, t1 := l.Snapshot()
	require.False(t, t1.IsZero())

	time.Sleep(2 * time.Millisecond)
	l.Set(0, 0, 0) // same numbers
	_, _, _, t2 := l.Snapshot()
	require.True(t, t2.After(t1), "updated_at should advance on every Set")
}
