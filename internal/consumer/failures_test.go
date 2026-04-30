package consumer_test

import (
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/consumer"
	"github.com/stretchr/testify/require"
)

func TestFailureTracker(t *testing.T) {
	ft := consumer.NewFailureTracker(5 * time.Minute)

	// 3 successes from 3 sources — ratio 0, distinct 3.
	ft.Record(1, false)
	ft.Record(2, false)
	ft.Record(3, false)
	failed, distinct, ratio := ft.FailureRatio()
	require.Equal(t, 0, failed)
	require.Equal(t, 3, distinct)
	require.InDelta(t, 0.0, ratio, 0.001)

	// Add 3 failures — ratio 0.5, distinct still 3.
	ft.Record(1, true)
	ft.Record(2, true)
	ft.Record(3, true)
	failed, distinct, ratio = ft.FailureRatio()
	require.Equal(t, 3, failed)
	require.Equal(t, 3, distinct)
	require.InDelta(t, 0.5, ratio, 0.001)
	require.Equal(t, 6, ft.AttemptCount())
}
