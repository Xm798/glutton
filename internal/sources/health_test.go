package sources_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/sources"
	"github.com/stretchr/testify/require"
)

func TestCooldownExponential(t *testing.T) {
	// Cooldown formula: 2^(consecutive_failures-1) minutes, capped at 60.
	cases := []struct {
		fails int
		want  time.Duration
	}{
		{1, 1 * time.Minute},
		{2, 2 * time.Minute},
		{3, 4 * time.Minute},
		{6, 32 * time.Minute},
		{7, 60 * time.Minute},
		{20, 60 * time.Minute},
	}
	for _, tc := range cases {
		require.Equalf(t, tc.want, sources.CooldownFor(tc.fails), "fails=%d", tc.fails)
	}
}

func TestPoolPickWeighted(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	pool := sources.NewPool([]sources.Candidate{
		{ID: 1, Weight: 1, CooldownUntil: time.Time{}},
		{ID: 2, Weight: 9, CooldownUntil: time.Time{}},
	}, rand.New(rand.NewSource(42)))

	counts := map[int64]int{}
	for i := 0; i < 1000; i++ {
		c, ok := pool.Pick(now, -1)
		require.True(t, ok)
		counts[c.ID]++
	}
	// With weights 1:9, ID 2 should win ~90%. Allow generous tolerance.
	require.Greater(t, counts[2], counts[1]*5, "id=2 should dominate, got %v", counts)
}

func TestPoolSkipsCooldown(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	pool := sources.NewPool([]sources.Candidate{
		{ID: 1, Weight: 1, CooldownUntil: now.Add(time.Hour)},
		{ID: 2, Weight: 1, CooldownUntil: time.Time{}},
	}, rand.New(rand.NewSource(1)))

	for i := 0; i < 50; i++ {
		c, ok := pool.Pick(now, -1)
		require.True(t, ok)
		require.Equal(t, int64(2), c.ID)
	}
}

func TestPoolAvoidsRepeat(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	pool := sources.NewPool([]sources.Candidate{
		{ID: 1, Weight: 1},
		{ID: 2, Weight: 1},
	}, rand.New(rand.NewSource(7)))

	first, ok := pool.Pick(now, -1)
	require.True(t, ok)
	second, ok := pool.Pick(now, first.ID)
	require.True(t, ok)
	require.NotEqual(t, first.ID, second.ID)
}

func TestPoolEmptyWhenAllOnCooldown(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	pool := sources.NewPool([]sources.Candidate{
		{ID: 1, Weight: 1, CooldownUntil: now.Add(time.Hour)},
	}, rand.New(rand.NewSource(1)))
	_, ok := pool.Pick(now, -1)
	require.False(t, ok)
}
