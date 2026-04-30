package scheduler

import (
	"context"
	"fmt"
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
	cfg Config
}

func New(cfg Config) *Scheduler {
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().In(cfg.Windows.loc) }
	}
	if cfg.TickInterval == 0 {
		cfg.TickInterval = time.Second
	}
	return &Scheduler{cfg: cfg}
}

// Tick re-evaluates state once. Exposed for unit tests; Run calls it on a timer.
func (s *Scheduler) Tick() {
	now := s.cfg.Now()

	// Quota checks take highest priority; only transition + notify on state change.
	if s.cfg.DailyQuotaBytes > 0 && s.cfg.BytesUsedDaily() >= s.cfg.DailyQuotaBytes {
		if s.cfg.State.Get() != QuotaReached {
			_ = s.cfg.State.QuotaReached()
			if s.cfg.OnQuotaReached != nil {
				s.cfg.OnQuotaReached("daily")
			}
		}
		return
	}
	if s.cfg.MonthlyQuotaBytes > 0 && s.cfg.BytesUsedMonthly() >= s.cfg.MonthlyQuotaBytes {
		if s.cfg.State.Get() != QuotaReached {
			_ = s.cfg.State.QuotaReached()
			if s.cfg.OnQuotaReached != nil {
				s.cfg.OnQuotaReached("monthly")
			}
		}
		return
	}

	if s.cfg.Windows.Contains(now) {
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
