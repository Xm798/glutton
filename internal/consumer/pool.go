package consumer

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

type PoolConfig struct {
	Workers  int
	Client   *http.Client
	Limiter  *rate.Limiter
	Provider func(ctx context.Context) (Job, bool) // false = no job available; worker idles briefly
	// OnProgress fires per chunk as bytes arrive. OnResult fires once at job end.
	OnProgress func(bytes int64)
	OnResult   func(j Job, bytes int64, rtt time.Duration, err error)
}

// Pool runs N download workers under a single limiter. The active worker
// count is dynamic via Resize; all mutation of the worker registry happens
// under p.mu, including spawn / shrink / Start. p.target is the requested
// active count, p.stopFns mirrors actually-spawned goroutines.
type Pool struct {
	cfg     PoolConfig
	wg      sync.WaitGroup
	d       *Downloader
	mu      sync.Mutex
	rootCtx context.Context
	started bool
	target  int                  // desired worker count under p.mu
	stopFns []context.CancelFunc // one per active worker; len == active count
	current atomic.Int32         // observable; updated by spawned goroutines
	// runCtx scopes every in-flight download. It is cancelled when the service
	// leaves the Running state (AbortInFlight) and replaced with a fresh live
	// context when it returns (ResumeInFlight), so an active drain stops at once
	// without tearing down the idle workers. Guarded by p.mu.
	runCtx    context.Context
	runCancel context.CancelFunc
}

func NewPool(cfg PoolConfig) *Pool {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	return &Pool{cfg: cfg, d: NewDownloader(cfg.Client, cfg.Limiter), target: cfg.Workers}
}

func (p *Pool) Start(ctx context.Context) {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return
	}
	p.started = true
	p.rootCtx = ctx
	p.runCtx, p.runCancel = context.WithCancel(ctx)
	want := p.target
	p.spawnLocked(want)
	p.mu.Unlock()
}

// AbortInFlight cancels every active download so bandwidth stops the moment the
// service leaves the Running state (pause / quota / window close). Idle workers
// keep running; the run context stays cancelled until ResumeInFlight refreshes
// it, so a job picked in a lost race with the transition is cancelled too.
// Safe to call before Start (no-op) and from a scheduler transition hook.
func (p *Pool) AbortInFlight() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.runCancel != nil {
		p.runCancel()
	}
}

// ResumeInFlight installs a fresh, live run context so downloads may start again
// after the service returns to Running.
func (p *Pool) ResumeInFlight() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.started {
		return
	}
	if p.runCancel != nil {
		p.runCancel()
	}
	p.runCtx, p.runCancel = context.WithCancel(p.rootCtx)
}

// currentRun returns the live run context for a new job, falling back to a
// background context before Start has wired one up.
func (p *Pool) currentRun() context.Context {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.runCtx == nil {
		return context.Background()
	}
	return p.runCtx
}

func (p *Pool) Wait() { p.wg.Wait() }

// Workers returns the current active worker count.
func (p *Pool) Workers() int { return int(p.current.Load()) }

// Resize adjusts the active worker count to n. Concurrent calls are safe:
// every state mutation happens under p.mu, and per-worker cancel funcs are
// only invoked outside the lock to avoid deadlocking with workers that
// might (in some imagined extension) call back. Workers in mid-download
// finish their current job before exiting because the cancel propagates
// only via context, which the Downloader honours after the next
// limiter.WaitN or read tick.
func (p *Pool) Resize(n int) {
	if n <= 0 {
		n = 1
	}
	p.mu.Lock()
	p.target = n
	if !p.started {
		// Pool not started yet: target is enough; Start will use it.
		p.mu.Unlock()
		return
	}
	cur := len(p.stopFns)
	var toCancel []context.CancelFunc
	switch {
	case n == cur:
		// no-op
	case n > cur:
		p.spawnLocked(n - cur)
	default:
		// shrink: detach cancel funcs of the trailing workers under the lock
		// so further Resize calls operate on a consistent slice.
		toCancel = append(toCancel, p.stopFns[n:]...)
		p.stopFns = p.stopFns[:n]
	}
	p.mu.Unlock()
	for _, cancel := range toCancel {
		cancel()
	}
}

// spawnLocked must be called with p.mu held and p.started == true.
func (p *Pool) spawnLocked(n int) {
	if n <= 0 || p.rootCtx == nil {
		return
	}
	for i := 0; i < n; i++ {
		ctx, cancel := context.WithCancel(p.rootCtx)
		p.stopFns = append(p.stopFns, cancel)
		p.current.Add(1)
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			defer p.current.Add(-1)
			p.runWorker(ctx)
		}()
	}
}

func (p *Pool) runWorker(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		job, ok := p.cfg.Provider(ctx)
		if !ok {
			// No job — idle briefly to avoid a hot loop.
			select {
			case <-ctx.Done():
				return
			case <-timeAfter(idleBackoff):
			}
			continue
		}
		// Scope the download to the worker context AND the run context, so it
		// aborts both on shutdown/resize and on leaving Running. AfterFunc fires
		// immediately if the run context was already cancelled (lost race with a
		// pause), closing the gap between Provider and the download start.
		jobCtx, jobCancel := context.WithCancel(ctx)
		stop := context.AfterFunc(p.currentRun(), jobCancel)
		n, rtt, err := p.d.Run(jobCtx, job, p.cfg.OnProgress)
		stop()
		jobCancel()
		if p.cfg.OnResult != nil {
			p.cfg.OnResult(job, n, rtt, err)
		}
	}
}
