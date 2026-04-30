package api_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/api"
	"github.com/cyrus/glutton/internal/events"
	"github.com/stretchr/testify/require"
)

func TestSSEStreamsEvents(t *testing.T) {
	bus := events.NewBus(8)
	defer bus.Close()
	srv := api.New(api.Deps{Bus: bus})

	httpsrv := httptest.NewServer(srv)
	defer httpsrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, httpsrv.URL+"/api/events", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Give the SSE handler a moment to subscribe before publishing.
	go func() {
		time.Sleep(150 * time.Millisecond)
		bus.Publish(events.Event{Type: "service_started", Message: "hi"})
	}()

	r := bufio.NewReader(resp.Body)
	deadline := time.Now().Add(time.Second)
	var got string
	for time.Now().Before(deadline) {
		line, err := r.ReadString('\n')
		if err != nil {
			break
		}
		got += line
		if strings.Contains(got, "service_started") {
			return
		}
	}
	t.Fatalf("did not receive event in stream, got: %q", got)
}
