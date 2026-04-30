package consumer_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/consumer"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func bigBodyServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		buf := make([]byte, 64*1024)
		// ~5 MB of data.
		for i := 0; i < 80; i++ {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}))
}

func TestDownloaderRespectsRateLimit(t *testing.T) {
	srv := bigBodyServer(t)
	t.Cleanup(srv.Close)

	// 1 MB/s cap. Burst of 1 MB. Stop after ~3s.
	lim := rate.NewLimiter(rate.Limit(1<<20), 1<<20)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)

	d := consumer.NewDownloader(http.DefaultClient, lim)
	start := time.Now()
	n, _, err := d.Run(ctx, consumer.Job{URL: srv.URL, UserAgent: "glutton-test/1.0"})
	elapsed := time.Since(start)

	require.True(t, err == nil || errors.Is(err, context.DeadlineExceeded), "err=%v", err)
	// Theoretical max in 3s at 1 MB/s = ~3 MB plus initial 1MB burst. Allow generous slack for scheduling.
	require.LessOrEqual(t, n, int64(4.5*float64(1<<20)), "downloaded too many bytes: %d", n)
	require.Greater(t, n, int64(1<<20), "downloaded too few bytes: %d (elapsed=%s)", n, elapsed)
}
