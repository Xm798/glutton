package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cyrus/glutton/internal/api"
	"github.com/cyrus/glutton/internal/config"
	"github.com/cyrus/glutton/internal/consumer"
	"github.com/cyrus/glutton/internal/events"
	"github.com/cyrus/glutton/internal/notify"
	"github.com/cyrus/glutton/internal/scheduler"
	"github.com/cyrus/glutton/internal/sources"
	"github.com/cyrus/glutton/internal/store"
	"github.com/cyrus/glutton/internal/version"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	logger.Info("starting glutton", "version", version.Version, "commit", version.Commit)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("mkdir data: %w", err)
	}

	loc, err := time.LoadLocation(cfg.TZ)
	if err != nil {
		return fmt.Errorf("load tz: %w", err)
	}

	db, err := store.Open(cfg.DBPath())
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close(db)

	rt := loadRuntime(context.Background(), db)
	logger.Info("runtime config",
		"daily_gb", rt.DailyQuotaGB,
		"monthly_gb", rt.MonthlyQuotaGB,
		"rate_mbps", rt.MaxRateMBps,
		"workers", rt.MaxConcurrent,
		"windows", rt.Windows)

	bus := events.NewBus(64)
	defer bus.Close()

	failureTracker := consumer.NewFailureTracker(5 * time.Minute)
	var lastMassFailureAt atomic.Int64 // unix seconds; debounce to at most once per 5 minutes

	state := scheduler.NewState()
	live := &api.LiveCounters{}

	sourcePool := buildSourcePool(context.Background(), db, loc)

	var todayBytes, monthBytes atomic.Int64

	// Recover counters from DB so a restart mid-day doesn't wipe Today / This
	// month while the per-source totals (DB-backed) keep marching.
	{
		now := time.Now()
		if d, err := store.SumTrafficSince(db, store.DayStart(now, loc)); err == nil {
			todayBytes.Store(d)
		}
		if m, err := store.SumTrafficSince(db, store.MonthStart(now, loc)); err == nil {
			monthBytes.Store(m)
		}
		logger.Info("seeded counters", "today_bytes", todayBytes.Load(), "month_bytes", monthBytes.Load())
	}

	var lastPickedSourceID atomic.Int64
	lastPickedSourceID.Store(-1)

	limiter := rate.NewLimiter(
		rate.Limit(rt.MaxRateMBps*1024*1024),
		int(rt.MaxRateMBps*1024*1024),
	)

	consumerPool := consumer.NewPool(consumer.PoolConfig{
		Workers: rt.MaxConcurrent,
		Client:  &http.Client{Timeout: 0},
		Limiter: limiter,
		Provider: func(ctx context.Context) (consumer.Job, bool) {
			if state.Get() != scheduler.Running {
				return consumer.Job{}, false
			}
			c, ok := sourcePool.Pick(time.Now().In(loc), lastPickedSourceID.Load())
			if !ok {
				return consumer.Job{}, false
			}
			lastPickedSourceID.Store(c.ID)
			return consumer.Job{
				SourceID:  uint(c.ID),
				URL:       c.URL,
				UserAgent: pickUA(c.UA, rt.DefaultUA),
			}, true
		},
		OnProgress: func(n int64) {
			todayBytes.Add(n)
			monthBytes.Add(n)
		},
		OnResult: func(j consumer.Job, n int64, rtt time.Duration, err error) {
			if n > 0 {
				_ = store.AddTraffic(db, store.HourBucket(time.Now().In(loc)), j.SourceID, n)
				api.BytesDownloadedTotal.WithLabelValues(fmt.Sprint(j.SourceID)).Add(float64(n))
				if rtt > 0 {
					api.SourceRTTSeconds.WithLabelValues(fmt.Sprint(j.SourceID)).Observe(rtt.Seconds())
				}
			}
			if err != nil {
				bus.Publish(events.Event{
					Type: "source_error", Level: "warn",
					Message: err.Error(),
					Data:    map[string]any{"source_id": j.SourceID},
				})
			}
			// Rolling-window mass-failure detection (spec §6.5).
			// Gate on ≥4 distinct sources attempted to suppress noise; debounce to once per 5 min.
			failureTracker.Record(j.SourceID, err != nil)
			if failed, distinct, ratio := failureTracker.FailureRatio(); failed >= 4 && distinct >= 4 && ratio >= 0.5 {
				nowSec := time.Now().Unix()
				last := lastMassFailureAt.Load()
				if nowSec-last >= 300 && lastMassFailureAt.CompareAndSwap(last, nowSec) {
					bus.Publish(events.Event{
						Type:    "sources_mass_failure",
						Level:   "error",
						Message: fmt.Sprintf("mass failure: %d/%d attempts failed across %d sources in last 5min", failed, failureTracker.AttemptCount(), distinct),
					})
				}
			}
		},
	})

	windows, err := scheduler.ParseWindows(rt.Windows, loc)
	if err != nil {
		return fmt.Errorf("parse windows: %w", err)
	}
	sched := scheduler.New(scheduler.Config{
		State:             state,
		Windows:           windows,
		DailyQuotaBytes:   int64(rt.DailyQuotaGB) * 1024 * 1024 * 1024,
		MonthlyQuotaBytes: int64(rt.MonthlyQuotaGB) * 1024 * 1024 * 1024,
		BytesUsedDaily:    func() int64 { return todayBytes.Load() },
		BytesUsedMonthly:  func() int64 { return monthBytes.Load() },
		Now:               func() time.Time { return time.Now().In(loc) },
		OnQuotaReached: func(scope string) {
			bus.Publish(events.Event{
				Type:    "quota_reached_" + scope,
				Level:   "warn",
				Message: "quota reached: " + scope,
			})
		},
	})

	notifier := notify.New(notify.Config{
		URLs:            rt.NotifierURLs,
		SubscribedTypes: rt.SubscribedEvents,
		PersistEvent: func(_ context.Context, e events.Event) error {
			return store.InsertEvent(db, &store.Event{
				Ts: e.TS.Unix(), Level: e.Level, Type: e.Type, Message: e.Message,
			})
		},
	})

	srv := api.New(api.Deps{
		Store: db, Live: live, State: state, Bus: bus, Loc: loc,
		Burst: &burstImpl{state: state, bytesUsed: todayBytes.Load, bus: bus},
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	api.ActiveWorkers.Set(float64(rt.MaxConcurrent))

	go sched.Run(ctx)
	go notifier.Run(ctx, bus)
	<-notifier.Ready() // ensure subscriber is registered before any publish
	consumerPool.Start(ctx)
	go runRetentionAndResets(ctx, db, loc, &todayBytes, &monthBytes, state, bus)
	go runRateSampler(ctx, &todayBytes, &monthBytes, live)
	bus.Publish(events.Event{Type: "service_started", Level: "info", Message: "service up"})

	httpSrv := &http.Server{Addr: cfg.Listen, Handler: srv}
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http", "err", err)
			cancel()
		}
	}()
	logger.Info("listening", "addr", cfg.Listen)

	<-ctx.Done()
	logger.Info("shutting down")
	bus.Publish(events.Event{Type: "service_stopped", Level: "info", Message: "service down"})
	shutdownCtx, sdCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer sdCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	consumerPool.Wait()
	return nil
}

// ---- runtime config helpers ----

type runtimeConfig struct {
	DailyQuotaGB     int      `json:"daily_quota_gb"`
	MonthlyQuotaGB   int      `json:"monthly_quota_gb"`
	MaxRateMBps      int      `json:"max_rate_mbps"`
	MaxConcurrent    int      `json:"max_concurrent"`
	Windows          []string `json:"time_windows"`
	DefaultUA        string   `json:"default_ua"`
	NotifierURLs     []string `json:"notifier_urls"`
	SubscribedEvents []string `json:"subscribed_events"`
}

func loadRuntime(ctx context.Context, db *gorm.DB) runtimeConfig {
	rc := runtimeConfig{
		MaxRateMBps:      10,
		MaxConcurrent:    4,
		Windows:          []string{"* 0-6 * * *"},
		DefaultUA:        "Mozilla/5.0 (compatible; glutton/1.0)",
		SubscribedEvents: []string{"quota_reached_daily", "quota_reached_monthly", "sources_mass_failure"},
	}
	tryDecode := func(key string, dst any) {
		if v, err := store.GetConfig(db, key); err == nil {
			_ = json.Unmarshal([]byte(v), dst)
		}
	}
	tryDecode("daily_quota_gb", &rc.DailyQuotaGB)
	tryDecode("monthly_quota_gb", &rc.MonthlyQuotaGB)
	tryDecode("max_rate_mbps", &rc.MaxRateMBps)
	tryDecode("max_concurrent", &rc.MaxConcurrent)
	tryDecode("time_windows", &rc.Windows)
	tryDecode("default_ua", &rc.DefaultUA)
	tryDecode("notifier_urls", &rc.NotifierURLs)
	tryDecode("subscribed_events", &rc.SubscribedEvents)
	_ = ctx // reserved for future per-key context plumbing
	return rc
}

func buildSourcePool(ctx context.Context, db *gorm.DB, loc *time.Location) *sources.Pool {
	rows, _ := store.ListEnabledSources(db)
	if len(rows) == 0 {
		// First-run seed: import builtins.
		bs, _ := sources.LoadBuiltins()
		for _, b := range bs {
			_ = store.CreateSource(db, &store.Source{
				Name: b.Name, URL: b.URL, UA: b.UA,
				Enabled: true, Weight: b.Weight,
			})
		}
		rows, _ = store.ListEnabledSources(db)
	}
	cands := make([]sources.Candidate, 0, len(rows))
	for _, r := range rows {
		cands = append(cands, sources.Candidate{
			ID: int64(r.ID), Name: r.Name, URL: r.URL, UA: r.UA,
			Weight:        r.Weight,
			CooldownUntil: time.Unix(r.CooldownUntil, 0),
		})
	}
	_ = ctx
	_ = loc
	return sources.NewPool(cands, rand.New(rand.NewSource(time.Now().UnixNano())))
}

func pickUA(perSource, def string) string {
	if perSource != "" {
		return perSource
	}
	return def
}

// burstImpl runs the scheduler regardless of cron window until the minute or
// byte cap hits (cap of 0 = no limit on that dimension; the handler enforces
// "at least one non-zero"). Quotas still apply via scheduler Tick.
//
// Generation token: a new Burst cancels the previous goroutine; only the
// latest generation may Deactivate, otherwise a stale goroutine waking after
// supersession would undo the new burst's Activate.
type burstImpl struct {
	state     *scheduler.State
	bytesUsed func() int64
	bus       *events.Bus

	mu     sync.Mutex
	gen    int64
	cancel context.CancelFunc
}

func (b *burstImpl) Burst(minutes int, bytes int64) {
	b.mu.Lock()
	if b.cancel != nil {
		b.cancel()
	}
	b.gen++
	myGen := b.gen
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel
	b.mu.Unlock()

	_ = b.state.Activate()
	startBytes := b.bytesUsed()
	if b.bus != nil {
		b.bus.Publish(events.Event{
			Type:    "burst_started",
			Level:   "info",
			Message: "manual burst started",
			Data:    map[string]any{"minutes": minutes, "bytes": bytes},
		})
	}

	go func() {
		var timeoutC <-chan time.Time
		if minutes > 0 {
			tm := time.NewTimer(time.Duration(minutes) * time.Minute)
			defer tm.Stop()
			timeoutC = tm.C
		}
		var pollC <-chan time.Time
		if bytes > 0 {
			poll := time.NewTicker(500 * time.Millisecond)
			defer poll.Stop()
			pollC = poll.C
		}

		reason := "expired"
		for done := false; !done; {
			if bytes > 0 && b.bytesUsed()-startBytes >= bytes {
				reason = "bytes_cap"
				break
			}
			select {
			case <-ctx.Done():
				reason = "superseded"
				done = true
			case <-timeoutC:
				done = true
			case <-pollC:
			}
		}

		b.mu.Lock()
		isLatest := b.gen == myGen
		if isLatest {
			b.cancel = nil
		}
		b.mu.Unlock()
		if isLatest {
			_ = b.state.Deactivate()
			if b.bus != nil {
				b.bus.Publish(events.Event{
					Type:    "burst_ended",
					Level:   "info",
					Message: "manual burst ended: " + reason,
					Data: map[string]any{
						"reason": reason,
						"bytes":  b.bytesUsed() - startBytes,
					},
				})
			}
		}
	}()
}

// runRateSampler reports current_rate_bps as delta(today-bytes)/elapsed over
// a sliding 5s window, sampled at 1Hz. OnProgress feeds today-bytes per chunk
// so this reflects active downloads, not just completed jobs.
func runRateSampler(ctx context.Context, today, month *atomic.Int64, live *api.LiveCounters) {
	const window = 5
	type sample struct {
		ts    time.Time
		bytes int64
	}
	ring := make([]sample, 0, window+1)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			cur := today.Load()
			// In-place head-shift keeps cap bounded; ring = ring[1:] would leak.
			cutoff := now.Add(-time.Duration(window) * time.Second)
			drop := 0
			for drop < len(ring) && ring[drop].ts.Before(cutoff) {
				drop++
			}
			if drop > 0 {
				if drop < len(ring) {
					copy(ring, ring[drop:])
				}
				ring = ring[:len(ring)-drop]
			}
			ring = append(ring, sample{ts: now, bytes: cur})
			var rate int64
			if len(ring) >= 2 {
				oldest := ring[0]
				if elapsed := now.Sub(oldest.ts).Seconds(); elapsed > 0 {
					rate = int64(float64(cur-oldest.bytes) / elapsed)
				}
			}
			live.Set(rate, cur, month.Load())
			api.CurrentRateGauge.Set(float64(rate))
		}
	}
}

func runRetentionAndResets(
	ctx context.Context,
	db *gorm.DB,
	loc *time.Location,
	today, month *atomic.Int64,
	state *scheduler.State,
	bus *events.Bus,
) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	var lastDay, lastMonth int
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := time.Now().In(loc)
			d := now.YearDay()
			m := int(now.Month())
			if lastDay == 0 {
				lastDay, lastMonth = d, m
				continue
			}
			if d != lastDay {
				today.Store(0)
				_ = state.ResetQuota()
				bus.Publish(events.Event{Type: "daily_reset", Level: "info", Message: "daily quota reset"})
				lastDay = d
				// Purge buckets older than 30 days.
				_ = store.PurgeTrafficBefore(db, store.HourBucket(now.Add(-30*24*time.Hour)))
				// Purge events older than 90 days.
				_ = store.PurgeEventsBefore(db, now.Add(-90*24*time.Hour).Unix())
			}
			if m != lastMonth {
				month.Store(0)
				lastMonth = m
				bus.Publish(events.Event{Type: "monthly_reset", Level: "info", Message: "monthly quota reset"})
			}
		}
	}
}
