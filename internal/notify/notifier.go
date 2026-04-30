package notify

import (
	"context"
	"fmt"

	"github.com/containrrr/shoutrrr"
	"github.com/cyrus/glutton/internal/events"
)

// Sender dispatches a single notification to one URL.
// Production code uses shoutrrrSender; tests inject a recorder.
type Sender interface {
	Send(ctx context.Context, url, message string) error
}

type Config struct {
	URLs            []string
	SubscribedTypes []string
	Sender          Sender // nil → shoutrrr-backed sender
	PersistEvent    func(ctx context.Context, e events.Event) error
}

type Notifier struct {
	cfg    Config
	subSet map[string]struct{}
	// ready is closed once Run has subscribed to the bus, allowing callers
	// to gate on readiness before publishing events.
	ready chan struct{}
}

func New(cfg Config) *Notifier {
	subs := make(map[string]struct{}, len(cfg.SubscribedTypes))
	for _, t := range cfg.SubscribedTypes {
		subs[t] = struct{}{}
	}
	if cfg.Sender == nil {
		cfg.Sender = shoutrrrSender{}
	}
	return &Notifier{cfg: cfg, subSet: subs, ready: make(chan struct{})}
}

// Ready returns a channel that is closed once Run has subscribed to the bus.
// Useful for tests or callers that publish events immediately after starting Run.
func (n *Notifier) Ready() <-chan struct{} {
	return n.ready
}

func (n *Notifier) Run(ctx context.Context, bus *events.Bus) {
	ch := bus.Subscribe()
	close(n.ready)
	defer bus.Unsubscribe(ch)
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if n.cfg.PersistEvent != nil {
				_ = n.cfg.PersistEvent(ctx, e)
			}
			if _, want := n.subSet[e.Type]; !want {
				continue
			}
			msg := fmt.Sprintf("[glutton/%s] %s", e.Type, e.Message)
			for _, u := range n.cfg.URLs {
				_ = n.cfg.Sender.Send(ctx, u, msg)
			}
		}
	}
}

type shoutrrrSender struct{}

func (shoutrrrSender) Send(_ context.Context, url, message string) error {
	return shoutrrr.Send(url, message)
}
