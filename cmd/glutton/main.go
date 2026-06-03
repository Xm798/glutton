package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	// Embed the IANA tz database into the binary so containers built on
	// distroless/static (no zoneinfo on disk) can still resolve TZ values
	// like Asia/Shanghai. Adds ~450 KB; worth it for crash-free startup.
	_ "time/tzdata"

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

// byteOnlyBurstSafetyTimeout caps the State-side deadline of a burst that
// only specified a byte cap. Without it, a misconfigured source that
// 200-OKs but never delivers bytes could lock the scheduler in Running for
// 24h. 2h is long enough for any realistic byte-only burst (e.g. "drain
// 50 GB at 5 MB/s ≈ 2.8h" — operators tend to set minutes for those).
const byteOnlyBurstSafetyTimeout = 2 * time.Hour

// configDebounce coalesces a burst of config_updated events (e.g. multi-key
// PUT loops, tests, or replays) into a single hot-apply pass.
const configDebounce = 50 * time.Millisecond

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

	rtSnap := &atomic.Pointer[runtimeConfig]{}
	rtSnap.Store(&rt)

	bus := events.NewBus(64)
	defer bus.Close()
	// Seed the monotonic event-id counter from the persisted max so SSE/history
	// ids never collide with rows from prior runs. Best-effort: a failed query
	// (very unlikely on a freshly-opened sqlite) leaves the counter at 0, which
	// only matters if the DB happens to already contain ids — in that case the
	// frontend dedupe falls back to (TS, Type, Message).
	if maxID, err := store.MaxEventID(db); err == nil {
		bus.SetNextID(maxID)
	} else {
		logger.Warn("seed event id counter", "err", err)
	}

	failureTracker := consumer.NewFailureTracker(5 * time.Minute)
	var lastMassFailureAt atomic.Int64

	state := scheduler.NewState()
	// Assigned below; captured by the transition hook. The hook only fires once
	// the scheduler/server goroutines start (after this assignment), so the read
	// is safely ordered after the write.
	var consumerPool *consumer.Pool
	state.SetTransitionHook(func(from, to scheduler.Status) {
		bus.Publish(events.Event{
			Type:    events.TypeStateChanged,
			Level:   "info",
			Message: fmt.Sprintf("state %s -> %s", from, to),
			Data:    map[string]any{"from": from.String(), "to": to.String()},
		})
		if consumerPool == nil {
			return
		}
		// Leaving Running must stop bandwidth immediately by cancelling in-flight
		// downloads; returning to Running re-arms a live context for new jobs.
		if to == scheduler.Running {
			consumerPool.ResumeInFlight()
		} else {
			consumerPool.AbortInFlight()
		}
	})
	live := &api.LiveCounters{}

	sourcePool := buildSourcePool(db)

	var consecFailMu sync.Mutex
	consecFails := make(map[uint]int)

	reloadSourcesFromDB := func() {
		rows, err := store.ListEnabledSources(db)
		if err != nil {
			logger.Warn("reload sources", "err", err)
			return
		}
		cands := make([]sources.Candidate, 0, len(rows))
		for _, r := range rows {
			cands = append(cands, sources.Candidate{
				ID: int64(r.ID), Name: r.Name, URLs: r.URLs, UA: r.UA,
				Weight:        r.Weight,
				CooldownUntil: time.Unix(r.CooldownUntil, 0),
			})
		}
		sourcePool.Replace(cands)
	}

	var todayBytes, monthBytes atomic.Int64
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

	limiter := rate.NewLimiter(limitFor(rt.MaxRateMBps))

	consumerPool = consumer.NewPool(consumer.PoolConfig{
		Workers: rt.MaxConcurrent,
		Client:  newConsumerHTTPClient(),
		Limiter: limiter,
		Provider: func(ctx context.Context) (consumer.Job, bool) {
			if state.Get() != scheduler.Running {
				return consumer.Job{}, false
			}
			c, url, ok := sourcePool.Pick(time.Now().In(loc), lastPickedSourceID.Load())
			if !ok {
				return consumer.Job{}, false
			}
			lastPickedSourceID.Store(c.ID)
			cur := rtSnap.Load()
			return consumer.Job{
				SourceID:  uint(c.ID),
				URL:       url,
				UserAgent: pickUA(c.UA, cur.DefaultUA),
			}, true
		},
		OnProgress: func(n int64) {
			todayBytes.Add(n)
			monthBytes.Add(n)
		},
		OnResult: func(j consumer.Job, n int64, ttfb, elapsed time.Duration, err error) {
			// A download cancelled because the service left Running (pause / quota
			// / window close / shutdown) is neither a source success nor failure.
			// Persist the bytes actually drained — they count toward quota and must
			// stay consistent with the live counters — but skip RTT and all source
			// health bookkeeping so an operator action can't corrupt source stats.
			aborted := errors.Is(err, context.Canceled)
			if n > 0 {
				_ = store.AddTraffic(db, store.HourBucket(time.Now().In(loc)), j.SourceID, n)
				api.BytesDownloadedTotal.WithLabelValues(fmt.Sprint(j.SourceID)).Add(float64(n))
				if ttfb > 0 && !aborted {
					api.SourceRTTSeconds.WithLabelValues(fmt.Sprint(j.SourceID)).Observe(ttfb.Seconds())
				}
			}
			if aborted {
				return
			}
			handleSourceResult(db, j.SourceID, n, elapsed, err, &consecFailMu, consecFails, bus, reloadSourcesFromDB)
			if err != nil {
				bus.Publish(events.Event{
					Type: events.TypeSourceError, Level: "warn",
					Message: err.Error(),
					Data:    map[string]any{"source_id": j.SourceID},
				})
			}
			failureTracker.Record(j.SourceID, err != nil)
			if failed, distinct, ratio := failureTracker.FailureRatio(); failed >= 4 && distinct >= 4 && ratio >= 0.5 {
				nowSec := time.Now().Unix()
				last := lastMassFailureAt.Load()
				if nowSec-last >= 300 && lastMassFailureAt.CompareAndSwap(last, nowSec) {
					bus.Publish(events.Event{
						Type:    events.TypeSourcesMassFailure,
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
				EventID: e.ID,
				Ts:      e.TS.Unix(),
				Level:   e.Level,
				Type:    e.Type,
				Message: e.Message,
			})
		},
	})

	// Tight-buffered subscription with debounce: bursty config_updated traffic
	// (multi-key PUTs, replays) is collapsed into a single reload pass via
	// loadRuntime+Apply, which is already idempotent.
	configCh := bus.SubscribeBuffered(4)
	go applyConfigUpdates(configCh, db, rtSnap, limiter, sched, loc, consumerPool, notifier, logger)

	srv := api.New(api.Deps{
		Store: db, Live: live, State: state, Bus: bus, Loc: loc,
		Burst:           &burstImpl{state: state, bytesUsed: todayBytes.Load, bus: bus},
		SourcesReloader: reloaderFunc(reloadSourcesFromDB),
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	api.ActiveWorkers.Set(float64(rt.MaxConcurrent))

	go sched.Run(ctx)
	go notifier.Run(ctx, bus)
	<-notifier.Ready()
	consumerPool.Start(ctx)
	go runRetentionAndResets(ctx, db, loc, &todayBytes, &monthBytes, state, bus)
	go runRateSampler(ctx, &todayBytes, &monthBytes, live)
	bus.Publish(events.Event{Type: events.TypeServiceStarted, Level: "info", Message: "service up"})

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
	bus.Publish(events.Event{Type: events.TypeServiceStopped, Level: "info", Message: "service down"})
	shutdownCtx, sdCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer sdCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	consumerPool.Wait()
	return nil
}

type reloaderFunc func()

func (r reloaderFunc) Reload() { r() }

func handleSourceResult(
	db *gorm.DB, sourceID uint, bytes int64, elapsed time.Duration, err error,
	mu *sync.Mutex, consec map[uint]int,
	bus *events.Bus, reload func(),
) {
	if sourceID == 0 {
		return
	}
	if err == nil {
		var avg int64
		if bytes > 0 && elapsed > 0 {
			avg = int64(float64(bytes) / elapsed.Seconds())
		}
		mu.Lock()
		delete(consec, sourceID)
		mu.Unlock()
		_ = store.RecordSourceSuccess(db, sourceID, avg, time.Now().Unix())
		reload()
		return
	}
	mu.Lock()
	consec[sourceID]++
	n := consec[sourceID]
	mu.Unlock()
	cooldown := sources.CooldownFor(n)
	until := int64(0)
	if cooldown > 0 {
		until = time.Now().Add(cooldown).Unix()
	}
	_ = store.RecordSourceFailure(db, sourceID, err.Error(), until)
	if cooldown > 0 && bus != nil {
		bus.Publish(events.Event{
			Type:    events.TypeSourceCooldown,
			Level:   "warn",
			Message: fmt.Sprintf("source %d cooled down for %s after %d consecutive failures", sourceID, cooldown, n),
			Data: map[string]any{
				"source_id":            sourceID,
				"cooldown_seconds":     int64(cooldown.Seconds()),
				"consecutive_failures": n,
			},
		})
	}
	reload()
}

// applyConfigUpdates listens for config_updated events and re-reads the DB
// to obtain the canonical merged config (handles partial PUTs that touch
// only some keys). A debounce timer coalesces bursts so a multi-key PUT
// loop only triggers one reload pass; loadRuntime is idempotent so this is
// strictly an efficiency optimisation, not a correctness one.
func applyConfigUpdates(
	ch <-chan events.Event,
	db *gorm.DB,
	snap *atomic.Pointer[runtimeConfig],
	limiter *rate.Limiter,
	sched *scheduler.Scheduler,
	loc *time.Location,
	pool *consumer.Pool,
	notifier *notify.Notifier,
	logger *slog.Logger,
) {
	apply := func() {
		next := loadRuntime(context.Background(), db)
		prev := snap.Load()
		snap.Store(&next)

		if prev == nil || prev.MaxRateMBps != next.MaxRateMBps {
			lim, burst := limitFor(next.MaxRateMBps)
			limiter.SetLimit(lim)
			limiter.SetBurst(burst)
		}
		// Cron expressions are order-insensitive (the union of windows is
		// what matters), so an order-only reorder should not churn the
		// schedules. notifier URLs and subscribed types are sets too.
		if prev == nil || !stringSetsEqual(prev.Windows, next.Windows) {
			w, err := scheduler.ParseWindows(next.Windows, loc)
			if err != nil {
				logger.Warn("config_updated: bad windows; keeping previous", "err", err)
			} else {
				sched.SetWindows(w)
			}
		}
		if prev == nil ||
			prev.DailyQuotaGB != next.DailyQuotaGB ||
			prev.MonthlyQuotaGB != next.MonthlyQuotaGB {
			sched.SetQuotas(
				int64(next.DailyQuotaGB)*1024*1024*1024,
				int64(next.MonthlyQuotaGB)*1024*1024*1024,
			)
		}
		if prev == nil || prev.MaxConcurrent != next.MaxConcurrent {
			pool.Resize(next.MaxConcurrent)
			api.ActiveWorkers.Set(float64(next.MaxConcurrent))
		}
		if prev == nil || !stringSetsEqual(prev.NotifierURLs, next.NotifierURLs) {
			notifier.SetURLs(next.NotifierURLs)
		}
		if prev == nil || !stringSetsEqual(prev.SubscribedEvents, next.SubscribedEvents) {
			notifier.SetSubscribedTypes(next.SubscribedEvents)
		}

		logger.Info("config hot-applied",
			"daily_gb", next.DailyQuotaGB,
			"monthly_gb", next.MonthlyQuotaGB,
			"rate_mbps", next.MaxRateMBps,
			"workers", next.MaxConcurrent,
			"windows", next.Windows)
	}

	var debouncer *time.Timer
	defer func() {
		if debouncer != nil {
			debouncer.Stop()
		}
	}()
	for e := range ch {
		if e.Type != events.TypeConfigUpdated {
			continue
		}
		// Drain any same-type events queued behind us before scheduling work,
		// belt-and-braces alongside the debounce timer for cases where the
		// channel buffer is full and Publish is dropping.
		drainConfigUpdates(ch)
		if debouncer == nil {
			debouncer = time.AfterFunc(configDebounce, apply)
		} else {
			debouncer.Reset(configDebounce)
		}
	}
}

// drainConfigUpdates removes any already-buffered config_updated events from
// the channel without blocking. Non-config events are left untouched (this
// channel currently only receives config_updated, but stay defensive).
func drainConfigUpdates(ch <-chan events.Event) {
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				return
			}
			if e.Type != events.TypeConfigUpdated {
				// Drop on the floor: this goroutine doesn't act on other
				// types, and another subscriber will have received it.
				continue
			}
		default:
			return
		}
	}
}

// stringSetsEqual reports whether two string slices contain the same set of
// values (order-insensitive, duplicate-collapsed). The cron-window list, the
// notifier URL list, and the subscribed-types list are all semantically sets,
// so a reorder by the operator should not trigger a hot-reload.
func stringSetsEqual(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	sa := append([]string(nil), a...)
	sb := append([]string(nil), b...)
	sort.Strings(sa)
	sort.Strings(sb)
	sa = dedupSorted(sa)
	sb = dedupSorted(sb)
	if len(sa) != len(sb) {
		return false
	}
	for i := range sa {
		if sa[i] != sb[i] {
			return false
		}
	}
	return true
}

func dedupSorted(s []string) []string {
	if len(s) < 2 {
		return s
	}
	out := s[:1]
	for i := 1; i < len(s); i++ {
		if s[i] != out[len(out)-1] {
			out = append(out, s[i])
		}
	}
	return out
}

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

const defaultRateMBps = 10

// limitFor maps a MaxRateMBps setting to a token-bucket limit and burst.
// 0 means unlimited: rate.Inf makes WaitN return immediately and the burst is
// ignored. A negative value (corrupt input) falls back to the safe default cap
// so a bad value can never silently uncap bandwidth, independent of any
// upstream sanitization.
func limitFor(mbps int) (rate.Limit, int) {
	switch {
	case mbps == 0:
		return rate.Inf, 1 << 20
	case mbps < 0:
		mbps = defaultRateMBps
	}
	b := mbps * 1024 * 1024
	return rate.Limit(b), b
}

func loadRuntime(ctx context.Context, db *gorm.DB) runtimeConfig {
	rc := runtimeConfig{
		MaxRateMBps:   defaultRateMBps,
		MaxConcurrent: 4,
		Windows:       []string{"* 0-6 * * *"},
		DefaultUA:     "Mozilla/5.0 (compatible; glutton/1.0)",
		// source_cooldown is included by default so operators are notified
		// when a source enters back-off — it's a strong "something is wrong"
		// signal even though persistence happens unconditionally.
		SubscribedEvents: events.DefaultSubscribedTypes(),
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
	// 0 is a valid, explicit setting meaning "unlimited". A negative value is
	// nonsensical (corrupt row / bad API write); normalize it to the safe default
	// so the stored/displayed value matches what limitFor will enforce.
	if rc.MaxRateMBps < 0 {
		rc.MaxRateMBps = defaultRateMBps
	}
	if rc.MaxConcurrent <= 0 {
		rc.MaxConcurrent = 4
	}
	_ = ctx
	return rc
}

func buildSourcePool(db *gorm.DB) *sources.Pool {
	rows, _ := store.ListEnabledSources(db)
	if len(rows) == 0 {
		bs, _ := sources.LoadBuiltins()
		for _, b := range bs {
			_ = store.CreateSource(db, &store.Source{
				Name: b.Name, URLs: b.URLs, UA: b.UA,
				Enabled: true, Weight: b.Weight,
			})
		}
		rows, _ = store.ListEnabledSources(db)
	}
	cands := make([]sources.Candidate, 0, len(rows))
	for _, r := range rows {
		cands = append(cands, sources.Candidate{
			ID: int64(r.ID), Name: r.Name, URLs: r.URLs, UA: r.UA,
			Weight:        r.Weight,
			CooldownUntil: time.Unix(r.CooldownUntil, 0),
		})
	}
	return sources.NewPool(cands, rand.New(rand.NewSource(time.Now().UnixNano())))
}

// newConsumerHTTPClient returns the http.Client used by the download workers.
// The transport's DialContext is wrapped with sources.SafeDialerControl so a
// post-validation DNS rebinding to a private IP fails at SYN time instead of
// quietly succeeding.
func newConsumerHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Control:   sources.SafeDialerControl,
	}
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Transport: tr}
}

func pickUA(perSource, def string) string {
	if perSource != "" {
		return perSource
	}
	return def
}

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

	// State-side deadline: minutes win when set; otherwise fall back to a
	// hard 2h ceiling so a stuck byte cap can't pin Running indefinitely.
	deadline := time.Time{}
	switch {
	case minutes > 0:
		deadline = time.Now().Add(time.Duration(minutes) * time.Minute)
	case bytes > 0:
		deadline = time.Now().Add(byteOnlyBurstSafetyTimeout)
	}

	b.state.EndBurst()
	_ = b.state.Activate(deadline)
	startBytes := b.bytesUsed()
	if b.bus != nil {
		b.bus.Publish(events.Event{
			Type:    events.TypeBurstStarted,
			Level:   "info",
			Message: "manual burst started",
			Data:    map[string]any{"minutes": minutes, "bytes": bytes},
		})
	}

	go func() {
		var timeoutC <-chan time.Time
		// Time-cap channel: minutes if set, else the byte-only safety ceiling.
		switch {
		case minutes > 0:
			tm := time.NewTimer(time.Duration(minutes) * time.Minute)
			defer tm.Stop()
			timeoutC = tm.C
		case bytes > 0:
			tm := time.NewTimer(byteOnlyBurstSafetyTimeout)
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
				if bytes > 0 && minutes == 0 {
					reason = "safety_timeout"
				}
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
			b.state.EndBurst()
			_ = b.state.Deactivate()
			if b.bus != nil {
				b.bus.Publish(events.Event{
					Type:    events.TypeBurstEnded,
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
				bus.Publish(events.Event{Type: events.TypeDailyReset, Level: "info", Message: "daily quota reset"})
				lastDay = d
				_ = store.PurgeTrafficBefore(db, store.HourBucket(now.Add(-30*24*time.Hour)))
				_ = store.PurgeEventsBefore(db, now.Add(-90*24*time.Hour).Unix())
			}
			if m != lastMonth {
				month.Store(0)
				lastMonth = m
				bus.Publish(events.Event{Type: events.TypeMonthlyReset, Level: "info", Message: "monthly quota reset"})
			}
		}
	}
}
