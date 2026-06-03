package scheduler

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
)

type Windows struct {
	loc       *time.Location
	schedules []cron.Schedule
}

func ParseWindows(exprs []string, loc *time.Location) (*Windows, error) {
	if loc == nil {
		loc = time.UTC
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	out := make([]cron.Schedule, 0, len(exprs))
	for _, e := range exprs {
		s, err := parser.Parse(e)
		if err != nil {
			return nil, fmt.Errorf("cron %q: %w", e, err)
		}
		out = append(out, s)
	}
	return &Windows{loc: loc, schedules: out}, nil
}

// Contains returns true if t falls within any cron-scheduled minute. We ask
// each schedule for its next fire time after the start of the current minute
// and check if it lands on that same minute.
func (w *Windows) Contains(t time.Time) bool {
	if w == nil || len(w.schedules) == 0 {
		return false
	}
	tl := t.In(w.loc).Truncate(time.Minute)
	prev := tl.Add(-time.Second) // moment just before this minute
	for _, s := range w.schedules {
		next := s.Next(prev)
		if next.Equal(tl) {
			return true
		}
	}
	return false
}

type Config struct {
	State             *State
	Windows           *Windows
	DailyQuotaBytes   int64 // 0 = unlimited
	MonthlyQuotaBytes int64
	BytesUsedDaily    func() int64
	BytesUsedMonthly  func() int64
	Now               func() time.Time
	TickInterval      time.Duration      // default 1s
	OnQuotaReached    func(scope string) // optional; scope = "daily"|"monthly"
}

type Scheduler struct {
	cfg     Config
	windows atomic.Pointer[Windows] // hot-swappable for runtime config updates
	quotas  atomic.Pointer[quotaCaps]
}

type quotaCaps struct {
	daily   int64
	monthly int64
}

func New(cfg Config) *Scheduler {
	if cfg.Now == nil {
		loc := time.Local
		if cfg.Windows != nil && cfg.Windows.loc != nil {
			loc = cfg.Windows.loc
		}
		cfg.Now = func() time.Time { return time.Now().In(loc) }
	}
	if cfg.TickInterval == 0 {
		cfg.TickInterval = time.Second
	}
	s := &Scheduler{cfg: cfg}
	s.windows.Store(cfg.Windows)
	s.quotas.Store(&quotaCaps{daily: cfg.DailyQuotaBytes, monthly: cfg.MonthlyQuotaBytes})
	return s
}

// SetWindows hot-swaps the cron schedules. Safe to call from any goroutine.
func (s *Scheduler) SetWindows(w *Windows) {
	s.windows.Store(w)
}

// SetQuotas hot-swaps the daily/monthly byte caps. 0 = unlimited.
func (s *Scheduler) SetQuotas(daily, monthly int64) {
	s.quotas.Store(&quotaCaps{daily: daily, monthly: monthly})
}

// Tick re-evaluates state once. Exposed for unit tests; Run calls it on a timer.
func (s *Scheduler) Tick() {
	now := s.cfg.Now()
	caps := s.quotas.Load()

	// Quota checks take highest priority; only transition + notify on state change.
	if caps.daily > 0 && s.cfg.BytesUsedDaily() >= caps.daily {
		if s.cfg.State.Get() != QuotaReached {
			_ = s.cfg.State.QuotaReached()
			if s.cfg.OnQuotaReached != nil {
				s.cfg.OnQuotaReached("daily")
			}
		}
		return
	}
	if caps.monthly > 0 && s.cfg.BytesUsedMonthly() >= caps.monthly {
		if s.cfg.State.Get() != QuotaReached {
			_ = s.cfg.State.QuotaReached()
			if s.cfg.OnQuotaReached != nil {
				s.cfg.OnQuotaReached("monthly")
			}
		}
		return
	}

	// No cap is exceeded. If we were parked in QuotaReached (cap raised or
	// removed via config, or usage rolled over), release it so the window
	// check below can resume normal scheduling.
	if s.cfg.State.Get() == QuotaReached {
		_ = s.cfg.State.ResetQuota()
	}

	w := s.windows.Load()
	if w.Contains(now) {
		_ = s.cfg.State.Activate()
	} else {
		_ = s.cfg.State.Deactivate()
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	t := time.NewTicker(s.cfg.TickInterval)
	defer t.Stop()
	s.Tick()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick()
		}
	}
}
