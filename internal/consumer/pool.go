package consumer

import (
	"context"
	"net/http"
	"sync"
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

type Pool struct {
	cfg PoolConfig
	wg  sync.WaitGroup
	d   *Downloader
}

func NewPool(cfg PoolConfig) *Pool {
	if cfg.Workers <= 0 {
		cfg.Workers = 1
	}
	if cfg.Client == nil {
		cfg.Client = http.DefaultClient
	}
	return &Pool{cfg: cfg, d: NewDownloader(cfg.Client, cfg.Limiter)}
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.cfg.Workers; i++ {
		p.wg.Add(1)
		go p.runWorker(ctx)
	}
}

func (p *Pool) Wait() { p.wg.Wait() }

func (p *Pool) runWorker(ctx context.Context) {
	defer p.wg.Done()
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
		n, rtt, err := p.d.Run(ctx, job, p.cfg.OnProgress)
		if p.cfg.OnResult != nil {
			p.cfg.OnResult(job, n, rtt, err)
		}
	}
}
