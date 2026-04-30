package integration_test

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/consumer"
	"github.com/cyrus/glutton/internal/sources"
	"github.com/cyrus/glutton/internal/store"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestEndToEndRateCap(t *testing.T) {
	// Origin: serves 64 MB of zero bytes per request.
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		buf := make([]byte, 64*1024)
		for i := 0; i < 1024; i++ {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}))
	defer origin.Close()

	// In-memory DB + seed one source pointing at origin.
	db, err := store.Open(":memory:")
	require.NoError(t, err)
	defer store.Close(db)

	require.NoError(t, store.CreateSource(db, &store.Source{
		Name: "local", URL: origin.URL, UA: "test/1",
		Enabled: true, Weight: 1,
	}))

	rows, err := store.ListEnabledSources(db)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	cands := []sources.Candidate{{
		ID: int64(rows[0].ID), Name: rows[0].Name, URL: rows[0].URL, Weight: 1,
	}}
	pool := sources.NewPool(cands, rand.New(rand.NewSource(1)))

	// Cap: 2 MB/s with 1 MB burst.
	const capBps = 2 * 1024 * 1024
	limiter := rate.NewLimiter(rate.Limit(capBps), capBps/2)

	var bytesDrained atomic.Int64
	cp := consumer.NewPool(consumer.PoolConfig{
		Workers: 4,
		Client:  http.DefaultClient,
		Limiter: limiter,
		Provider: func(ctx context.Context) (consumer.Job, bool) {
			c, ok := pool.Pick(time.Now(), -1)
			if !ok {
				return consumer.Job{}, false
			}
			return consumer.Job{SourceID: uint(c.ID), URL: c.URL}, true
		},
		OnResult: func(_ consumer.Job, n int64, _ time.Duration, _ error) { bytesDrained.Add(n) },
	})

	// Run for 4s.
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	cp.Start(ctx)
	cp.Wait()

	// Theoretical max in 4s = capBps*4 + initial burst (capBps/2). Allow +10% slack.
	nominal := int64(capBps*4 + capBps/2)
	maxBytes := int64(float64(nominal) * 1.10)
	got := bytesDrained.Load()
	require.LessOrEqual(t, got, maxBytes,
		"got=%d bytes (~%.2f MB), exceeded cap+burst (max=%d, ~%.2f MB)",
		got, float64(got)/(1<<20), maxBytes, float64(maxBytes)/(1<<20))
	require.Greater(t, got, int64(capBps), "got too few bytes: %d (~%.2f MB)",
		got, float64(got)/(1<<20))

	t.Logf("drained %d bytes in 4s (cap=%d Bps, max=%d Bps)", got, capBps, maxBytes)
}
