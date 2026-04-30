package notify_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/events"
	"github.com/cyrus/glutton/internal/notify"
	"github.com/stretchr/testify/require"
)

type recorder struct{ count atomic.Int32 }

func (r *recorder) Send(_ context.Context, _ string, _ string) error {
	r.count.Add(1)
	return nil
}

func TestNotifierFiltersBySubscribedTypes(t *testing.T) {
	bus := events.NewBus(8)
	defer bus.Close()

	rec := &recorder{}
	cfg := notify.Config{
		URLs:            []string{"recorder://"},
		SubscribedTypes: []string{"quota_reached_daily"},
		Sender:          rec,
		PersistEvent:    func(_ context.Context, _ events.Event) error { return nil },
	}
	n := notify.New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go n.Run(ctx, bus)
	<-n.Ready() // wait until Run has subscribed before publishing

	bus.Publish(events.Event{Type: "service_started"})         // not subscribed
	bus.Publish(events.Event{Type: "quota_reached_daily", Message: "hi"})

	require.Eventually(t, func() bool { return rec.count.Load() == 1 },
		time.Second, 10*time.Millisecond)
}
