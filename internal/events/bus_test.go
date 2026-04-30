package events_test

import (
	"sync"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/events"
	"github.com/stretchr/testify/require"
)

func TestBusFanout(t *testing.T) {
	b := events.NewBus(8)
	defer b.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	got := make(chan events.Event, 4)
	for i := 0; i < 2; i++ {
		ch := b.Subscribe()
		go func() {
			defer wg.Done()
			select {
			case e := <-ch:
				got <- e
			case <-time.After(time.Second):
				t.Errorf("subscriber timed out")
			}
		}()
	}

	b.Publish(events.Event{Type: "service_started"})
	wg.Wait()
	close(got)

	count := 0
	for range got {
		count++
	}
	require.Equal(t, 2, count)
}

func TestBusDropsOnSlowSubscriber(t *testing.T) {
	b := events.NewBus(2)
	defer b.Close()

	_ = b.Subscribe() // never read
	for i := 0; i < 100; i++ {
		b.Publish(events.Event{Type: "noise"})
	}
	// No deadlock = pass.
}
