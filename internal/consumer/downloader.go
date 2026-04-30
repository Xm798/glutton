package consumer

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

type Job struct {
	SourceID  uint
	URL       string
	UserAgent string
}

type Downloader struct {
	client  *http.Client
	limiter *rate.Limiter
}

func NewDownloader(client *http.Client, limiter *rate.Limiter) *Downloader {
	if client == nil {
		client = http.DefaultClient
	}
	return &Downloader{client: client, limiter: limiter}
}

// Run streams the response body into io.Discard under the rate limiter.
// Returns bytes drained, TTFB (time to first byte), and any error.
// Returns nil error on context cancellation after partial drain.
func (d *Downloader) Run(ctx context.Context, j Job) (int64, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.URL, nil)
	if err != nil {
		return 0, 0, fmt.Errorf("new request: %w", err)
	}
	if j.UserAgent != "" {
		req.Header.Set("User-Agent", j.UserAgent)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, 0, fmt.Errorf("http status %d", resp.StatusCode)
	}

	// Capture TTFB start before WaitN to avoid inflating it with rate-limiter delay.
	ttfbStart := time.Now()
	var ttfb time.Duration
	firstRead := true

	// Drain in 64KB chunks; reserve from the limiter per chunk.
	const chunk = 64 * 1024
	buf := make([]byte, chunk)
	var total int64
	for {
		if err := d.limiter.WaitN(ctx, chunk); err != nil {
			return total, ttfb, nil // ctx canceled — partial drain, no error
		}
		n, rerr := io.ReadFull(resp.Body, buf)
		total += int64(n)
		if n > 0 {
			_, _ = io.Discard.Write(buf[:n]) // explicit discard for clarity
			if firstRead {
				ttfb = time.Since(ttfbStart)
				firstRead = false
			}
		}
		if rerr == io.EOF || rerr == io.ErrUnexpectedEOF {
			return total, ttfb, nil
		}
		if rerr != nil {
			if ctx.Err() != nil {
				return total, ttfb, nil
			}
			return total, ttfb, fmt.Errorf("read: %w", rerr)
		}
	}
}

const idleBackoff = 250 * time.Millisecond

// timeAfter is overridable in tests if we need deterministic timing.
var timeAfter = time.After
