package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/api"
	"github.com/cyrus/glutton/internal/consumer"
	"github.com/cyrus/glutton/internal/events"
	"github.com/cyrus/glutton/internal/scheduler"
	"github.com/cyrus/glutton/internal/sources"
	"github.com/cyrus/glutton/internal/store"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
	"gorm.io/gorm"
)

func newOrigin(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		buf := make([]byte, 64*1024)
		for i := 0; i < 1024; i++ {
			if _, err := w.Write(buf); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

type rig struct {
	t        *testing.T
	db       *gorm.DB
	bus      *events.Bus
	state    *scheduler.State
	limiter  *rate.Limiter
	pool     *sources.Pool
	sched    *scheduler.Scheduler
	consumer *consumer.Pool
	srv      *httptest.Server
	api      *api.Server
	today    *atomic.Int64
	month    *atomic.Int64
	reloadFn func()
	cancel   context.CancelFunc
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

func loadRuntime(db *gorm.DB) runtimeConfig {
	rc := runtimeConfig{
		MaxRateMBps:   2,
		MaxConcurrent: 2,
		Windows:       []string{"* * * * *"},
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
	if rc.MaxRateMBps <= 0 {
		rc.MaxRateMBps = 2
	}
	if rc.MaxConcurrent <= 0 {
		rc.MaxConcurrent = 2
	}
	return rc
}

func newRig(t *testing.T, originURL string, windows []string) *rig {
	t.Helper()
	db, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close(db) })

	require.NoError(t, store.CreateSource(db, &store.Source{
		Name: "primary", URLs: []string{originURL}, Enabled: true, Weight: 1,
	}))
	rows, _ := store.ListEnabledSources(db)
	cands := []sources.Candidate{}
	for _, r := range rows {
		cands = append(cands, sources.Candidate{
			ID: int64(r.ID), Name: r.Name, URLs: r.URLs,
			Weight:        r.Weight,
			CooldownUntil: time.Unix(r.CooldownUntil, 0),
		})
	}
	pool := sources.NewPool(cands, rand.New(rand.NewSource(1)))

	bus := events.NewBus(64)
	t.Cleanup(func() { bus.Close() })

	state := scheduler.NewState()
	live := &api.LiveCounters{}

	rc := loadRuntime(db)
	if len(windows) > 0 {
		rc.Windows = windows
	}

	loc := time.UTC
	w, err := scheduler.ParseWindows(rc.Windows, loc)
	require.NoError(t, err)

	limiter := rate.NewLimiter(rate.Limit(rc.MaxRateMBps*1024*1024), rc.MaxRateMBps*1024*1024)

	var today, month atomic.Int64
	var lastID atomic.Int64
	lastID.Store(-1)

	var consecMu sync.Mutex
	consec := make(map[uint]int)

	reload := func() {
		rows, _ := store.ListEnabledSources(db)
		cs := make([]sources.Candidate, 0, len(rows))
		for _, r := range rows {
			cs = append(cs, sources.Candidate{
				ID: int64(r.ID), Name: r.Name, URLs: r.URLs,
				Weight:        r.Weight,
				CooldownUntil: time.Unix(r.CooldownUntil, 0),
			})
		}
		pool.Replace(cs)
	}

	cp := consumer.NewPool(consumer.PoolConfig{
		Workers: rc.MaxConcurrent,
		Client:  http.DefaultClient,
		Limiter: limiter,
		Provider: func(ctx context.Context) (consumer.Job, bool) {
			if state.Get() != scheduler.Running {
				return consumer.Job{}, false
			}
			c, url, ok := pool.Pick(time.Now(), lastID.Load())
			if !ok {
				return consumer.Job{}, false
			}
			lastID.Store(c.ID)
			return consumer.Job{SourceID: uint(c.ID), URL: url}, true
		},
		OnProgress: func(n int64) {
			today.Add(n)
			month.Add(n)
		},
		OnResult: func(j consumer.Job, n int64, rtt time.Duration, err error) {
			if err == nil {
				consecMu.Lock()
				delete(consec, j.SourceID)
				consecMu.Unlock()
				if n > 0 && rtt > 0 {
					_ = store.RecordSourceSuccess(db, j.SourceID, int64(float64(n)/rtt.Seconds()), time.Now().Unix())
				}
				reload()
				return
			}
			consecMu.Lock()
			consec[j.SourceID]++
			cnt := consec[j.SourceID]
			consecMu.Unlock()
			cd := sources.CooldownFor(cnt)
			until := int64(0)
			if cd > 0 {
				until = time.Now().Add(cd).Unix()
			}
			_ = store.RecordSourceFailure(db, j.SourceID, err.Error(), until)
			reload()
		},
	})

	sched := scheduler.New(scheduler.Config{
		State: state, Windows: w,
		DailyQuotaBytes:   int64(rc.DailyQuotaGB) * 1024 * 1024 * 1024,
		MonthlyQuotaBytes: int64(rc.MonthlyQuotaGB) * 1024 * 1024 * 1024,
		BytesUsedDaily:    func() int64 { return today.Load() },
		BytesUsedMonthly:  func() int64 { return month.Load() },
		Now:               func() time.Time { return time.Now().UTC() },
		TickInterval:      100 * time.Millisecond,
	})

	apiSrv := api.New(api.Deps{
		Store: db, Live: live, State: state, Bus: bus, Loc: loc,
		Burst:           &testBurst{state: state, bytesUsed: today.Load, bus: bus},
		SourcesReloader: reloaderFn(reload),
	})
	httpSrv := httptest.NewServer(apiSrv)
	t.Cleanup(httpSrv.Close)

	configCh := bus.Subscribe()
	go func() {
		for e := range configCh {
			if e.Type != events.TypeConfigUpdated {
				continue
			}
			next := loadRuntime(db)
			limiter.SetLimit(rate.Limit(next.MaxRateMBps * 1024 * 1024))
			limiter.SetBurst(next.MaxRateMBps * 1024 * 1024)
			if w2, err := scheduler.ParseWindows(next.Windows, loc); err == nil {
				sched.SetWindows(w2)
			}
			sched.SetQuotas(
				int64(next.DailyQuotaGB)*1024*1024*1024,
				int64(next.MonthlyQuotaGB)*1024*1024*1024,
			)
			cp.Resize(next.MaxConcurrent)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go sched.Run(ctx)
	cp.Start(ctx)

	r := &rig{
		t: t, db: db, bus: bus, state: state, limiter: limiter, pool: pool,
		sched: sched, consumer: cp, srv: httpSrv, api: apiSrv,
		today: &today, month: &month, reloadFn: reload, cancel: cancel,
	}
	t.Cleanup(func() {
		cancel()
		cp.Wait()
	})
	return r
}

type reloaderFn func()

func (r reloaderFn) Reload() { r() }

type testBurst struct {
	state     *scheduler.State
	bytesUsed func() int64
	bus       *events.Bus
}

func (b *testBurst) Burst(minutes int, byteCap int64) {
	deadline := time.Time{}
	switch {
	case minutes > 0:
		deadline = time.Now().Add(time.Duration(minutes) * time.Minute)
	case byteCap > 0:
		deadline = time.Now().Add(24 * time.Hour)
	}
	b.state.EndBurst()
	_ = b.state.Activate(deadline)
	startBytes := b.bytesUsed()

	go func() {
		var timeoutC <-chan time.Time
		if minutes > 0 {
			tm := time.NewTimer(time.Duration(minutes) * time.Minute)
			defer tm.Stop()
			timeoutC = tm.C
		}
		var pollC <-chan time.Time
		if byteCap > 0 {
			tk := time.NewTicker(50 * time.Millisecond)
			defer tk.Stop()
			pollC = tk.C
		}
		for done := false; !done; {
			if byteCap > 0 && b.bytesUsed()-startBytes >= byteCap {
				break
			}
			select {
			case <-timeoutC:
				done = true
			case <-pollC:
			}
		}
		b.state.EndBurst()
		_ = b.state.Deactivate()
	}()
}

func putConfig(t *testing.T, baseURL string, body map[string]any) {
	t.Helper()
	buf, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPut, baseURL+"/api/config", bytes.NewReader(buf))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT /api/config: status=%d body=%s", resp.StatusCode, string(b))
	}
}

func TestB1RateLimitHotUpdate(t *testing.T) {
	origin := newOrigin(t)
	rig := newRig(t, origin.URL, []string{"* * * * *"})

	require.Eventually(t, func() bool { return rig.state.Get() == scheduler.Running },
		3*time.Second, 50*time.Millisecond)

	time.Sleep(1500 * time.Millisecond)
	mid := rig.today.Load()
	require.Greater(t, mid, int64(1<<20), "should have drained ≥1 MB at 2 MB/s")

	putConfig(t, rig.srv.URL, map[string]any{"max_rate_mbps": 8})
	time.Sleep(300 * time.Millisecond)

	startNew := rig.today.Load()
	time.Sleep(1500 * time.Millisecond)
	deltaNew := rig.today.Load() - startNew

	deltaOld := mid
	t.Logf("old window delta=%d (1.5s @2MBps), new window delta=%d (1.5s @8MBps)", deltaOld, deltaNew)
	require.Greater(t, deltaNew, deltaOld*2, "new rate should outpace old by ≥2x")
}

func TestB2BurstOverridesCronWindow(t *testing.T) {
	origin := newOrigin(t)
	rig := newRig(t, origin.URL, []string{"0 0 1 1 *"})

	require.Eventually(t, func() bool { return rig.state.Get() == scheduler.Idle },
		2*time.Second, 50*time.Millisecond)

	body := bytes.NewReader([]byte(`{"minutes":1}`))
	resp, err := http.Post(rig.srv.URL+"/api/control/burst", "application/json", body)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	require.Eventually(t, func() bool { return rig.state.Get() == scheduler.Running },
		2*time.Second, 50*time.Millisecond, "burst should flip state to Running despite cron miss")

	startBytes := rig.today.Load()
	time.Sleep(1200 * time.Millisecond)
	require.Greater(t, rig.today.Load()-startBytes, int64(64*1024),
		"burst should drain bytes even outside cron window")
}

func TestB3SourceCooldownAndHotInsert(t *testing.T) {
	healthy := newOrigin(t)
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(failing.Close)

	rig := newRig(t, failing.URL, []string{"* * * * *"})

	require.Eventually(t, func() bool { return rig.state.Get() == scheduler.Running },
		3*time.Second, 50*time.Millisecond)

	require.Eventually(t, func() bool {
		_, _, ok := rig.pool.Pick(time.Now(), -1)
		return !ok
	}, 5*time.Second, 50*time.Millisecond, "failing source should eventually be cooled down so Pick returns false")

	// httptest origins live on 127.0.0.1 which the API validator rejects;
	// insert the row directly into the store and trigger the reloader to
	// exercise the same hot-reload path the API handler uses.
	require.NoError(t, store.CreateSource(rig.db, &store.Source{
		Name: "healthy", URLs: []string{healthy.URL}, Enabled: true, Weight: 1,
	}))
	rig.reloadFn()

	require.Eventually(t, func() bool {
		_, url, ok := rig.pool.Pick(time.Now(), -1)
		return ok && url == healthy.URL
	}, 2*time.Second, 50*time.Millisecond, "newly added healthy source must be pickable without restart")
}
