package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/cyrus/glutton/internal/events"
	"github.com/labstack/echo/v4"
)

type sseHandlers struct {
	bus *events.Bus
}

func (h *sseHandlers) stream(c echo.Context) error {
	if h.bus == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "bus not configured")
	}
	w := c.Response()
	w.Header().Set(echo.HeaderContentType, "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	w.Flush()

	ch := h.bus.Subscribe()
	defer h.bus.Unsubscribe(ch)
	ctx := c.Request().Context()

	for {
		select {
		case <-ctx.Done():
			return nil
		case e, ok := <-ch:
			if !ok {
				return nil
			}
			payload, err := json.Marshal(e)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, payload); err != nil {
				return nil
			}
			w.Flush()
		}
	}
}
