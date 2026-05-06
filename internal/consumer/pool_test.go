package consumer_test

import (
	"context"
	"math/rand"
	"sync"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/consumer"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestWorkerPoolRunsAndStops(t *testing.T) {
	srv := bigBodyServer(t)
	t.Cleanup(srv.Close)

	var bytes atomic.Int64
	var jobs atomic.Int32
	provider := func(ctx context.Context) (consumer.Job, bool) {
		jobs.Add(1)
		return consumer.Job{URL: srv.URL, UserAgent: "ua"}, true
	}
	onResult := func(_ consumer.Job, n int64, _ time.Duration, _ error) {
		bytes.Add(n)
	}

	lim := rate.NewLimiter(rate.Limit(2<<20), 1<<20) // 2 MB/s
	pool := consumer.NewPool(consumer.PoolConfig{
		Workers:  2,
		Client:   http.DefaultClient,
		Limiter:  lim,
		Provider: provider,
		OnResult: onResult,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	t.Cleanup(cancel)

	pool.Start(ctx)
	pool.Wait()

	require.Greater(t, bytes.Load(), int64(1<<20))
	require.Greater(t, jobs.Load(), int32(0))
}

func TestWorkerPoolStopsImmediatelyOnCancel(t *testing.T) {
	srv := bigBodyServer(t)
	t.Cleanup(srv.Close)

	provider := func(ctx context.Context) (consumer.Job, bool) {
		return consumer.Job{URL: srv.URL}, true
	}
	pool := consumer.NewPool(consumer.PoolConfig{
		Workers:  4,
		Client:   http.DefaultClient,
		Limiter:  rate.NewLimiter(rate.Limit(10<<20), 1<<20),
		Provider: provider,
		OnResult: func(_ consumer.Job, _ int64, _ time.Duration, _ error) {},
	})

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)
	time.AfterFunc(100*time.Millisecond, cancel)

	done := make(chan struct{})
	go func() { pool.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pool did not stop within 2s of cancel")
	}
}

func TestWorkerPoolResizeUpAndDown(t *testing.T) {
	srv := bigBodyServer(t)
	t.Cleanup(srv.Close)

	provider := func(ctx context.Context) (consumer.Job, bool) {
		return consumer.Job{URL: srv.URL}, true
	}
	pool := consumer.NewPool(consumer.PoolConfig{
		Workers:  2,
		Client:   http.DefaultClient,
		Limiter:  rate.NewLimiter(rate.Limit(20<<20), 1<<20),
		Provider: provider,
		OnResult: func(_ consumer.Job, _ int64, _ time.Duration, _ error) {},
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool.Start(ctx)

	require.Eventually(t, func() bool { return pool.Workers() == 2 }, time.Second, 10*time.Millisecond)

	pool.Resize(5)
	require.Eventually(t, func() bool { return pool.Workers() == 5 }, time.Second, 10*time.Millisecond)

	pool.Resize(1)
	require.Eventually(t, func() bool { return pool.Workers() == 1 }, 3*time.Second, 20*time.Millisecond)

	cancel()
	pool.Wait()
}

// TestM1ResizeRaceUnderConcurrency hammers Resize from many goroutines while
// the pool is actively running workers. Run under -race; the data races on
// the original implementation (cfg.Workers writes from spawn, stopFns slice
// re-borrowed across lock-drop) showed up here.
func TestM1ResizeRaceUnderConcurrency(t *testing.T) {
	srv := bigBodyServer(t)
	t.Cleanup(srv.Close)

	pool := consumer.NewPool(consumer.PoolConfig{
		Workers:  3,
		Client:   http.DefaultClient,
		Limiter:  rate.NewLimiter(rate.Limit(50<<20), 1<<20),
		Provider: func(_ context.Context) (consumer.Job, bool) { return consumer.Job{URL: srv.URL}, true },
		OnResult: func(_ consumer.Job, _ int64, _ time.Duration, _ error) {},
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool.Start(ctx)

	const N = 32
	var wg sync.WaitGroup
	wg.Add(N)
	deadline := time.Now().Add(800 * time.Millisecond)
	for i := 0; i < N; i++ {
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(seed)))
			for time.Now().Before(deadline) {
				pool.Resize(1 + rng.Intn(8))
			}
		}(i)
	}
	wg.Wait()

	// Settle to a known size and verify.
	pool.Resize(2)
	require.Eventually(t, func() bool { return pool.Workers() == 2 },
		3*time.Second, 20*time.Millisecond, "settled worker count must converge to target")

	cancel()
	pool.Wait()
}

// TestM1ResizeBeforeStartTakesEffect verifies that Resize calls before
// Start() set the target, and Start() spawns the most-recently-requested
// number — exercising the !started branch added during the M-1 cleanup.
func TestM1ResizeBeforeStartTakesEffect(t *testing.T) {
	srv := bigBodyServer(t)
	t.Cleanup(srv.Close)

	pool := consumer.NewPool(consumer.PoolConfig{
		Workers:  1,
		Client:   http.DefaultClient,
		Limiter:  rate.NewLimiter(rate.Limit(20<<20), 1<<20),
		Provider: func(_ context.Context) (consumer.Job, bool) { return consumer.Job{URL: srv.URL}, true },
		OnResult: func(_ consumer.Job, _ int64, _ time.Duration, _ error) {},
	})
	pool.Resize(4)
	pool.Resize(6) // last one wins

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool.Start(ctx)

	require.Eventually(t, func() bool { return pool.Workers() == 6 },
		2*time.Second, 20*time.Millisecond)
	cancel()
	pool.Wait()
}

// TestM1DoubleStartIsNoop ensures a stray Start after the first one doesn't
// double-spawn workers.
func TestM1DoubleStartIsNoop(t *testing.T) {
	srv := bigBodyServer(t)
	t.Cleanup(srv.Close)
	pool := consumer.NewPool(consumer.PoolConfig{
		Workers:  3,
		Client:   http.DefaultClient,
		Limiter:  rate.NewLimiter(rate.Limit(20<<20), 1<<20),
		Provider: func(_ context.Context) (consumer.Job, bool) { return consumer.Job{URL: srv.URL}, true },
		OnResult: func(_ consumer.Job, _ int64, _ time.Duration, _ error) {},
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	pool.Start(ctx)
	pool.Start(ctx) // second call must be a no-op

	require.Eventually(t, func() bool { return pool.Workers() == 3 },
		2*time.Second, 20*time.Millisecond)
	cancel()
	pool.Wait()
}
