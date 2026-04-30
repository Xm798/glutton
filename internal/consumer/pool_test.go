package consumer_test

import (
	"context"
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
	onResult := func(_ consumer.Job, n int64, _ error) {
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
		OnResult: func(_ consumer.Job, _ int64, _ error) {},
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
