package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cyrus/glutton/internal/events"
	"github.com/cyrus/glutton/internal/sources"
	"github.com/cyrus/glutton/internal/store"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

// SourcesReloader is implemented by main.go and called after any CRUD/Toggle
// to keep the in-memory source pool in sync with the DB.
type SourcesReloader interface {
	Reload()
}

type sourcesHandlers struct {
	store    *gorm.DB
	bus      *events.Bus
	reloader SourcesReloader
}

type sourceIn struct {
	Name    string   `json:"name"`
	URLs    []string `json:"urls"`
	UA      string   `json:"ua"`
	Enabled bool     `json:"enabled"`
	Weight  int      `json:"weight"`
}

// cleanURLs trims each entry and drops blanks.
func cleanURLs(in []string) []string {
	out := make([]string, 0, len(in))
	for _, u := range in {
		if u = strings.TrimSpace(u); u != "" {
			out = append(out, u)
		}
	}
	return out
}

func (h *sourcesHandlers) reload() {
	if h.reloader != nil {
		h.reloader.Reload()
	}
}

// emit publishes an audit event. When a bus is wired, persistence is handled
// by the notifier's PersistEvent hook so the row carries the bus's monotonic
// EventID. Without a bus (unit tests) we fall back to a direct InsertEvent
// to keep the audit trail intact.
func (h *sourcesHandlers) emit(typ, msg string, data map[string]any) {
	if h.bus != nil {
		h.bus.Publish(events.Event{Type: typ, Level: "info", Message: msg, Data: data})
		return
	}
	if h.store != nil {
		_ = store.InsertEvent(h.store, &store.Event{
			Ts: time.Now().Unix(), Level: "info", Type: typ, Message: msg,
		})
	}
}

func (h *sourcesHandlers) list(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "store not configured")
	}
	rows, err := store.ListSources(h.store)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, rows)
}

func (h *sourcesHandlers) create(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "store not configured")
	}
	var in sourceIn
	if err := c.Bind(&in); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	in.URLs = cleanURLs(in.URLs)
	if err := sources.ValidateURLs(in.URLs); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if in.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if in.Weight <= 0 {
		in.Weight = 1
	}
	row := &store.Source{
		Name: in.Name, URLs: in.URLs, UA: in.UA,
		Enabled: in.Enabled, Weight: in.Weight,
	}
	if err := store.CreateSource(h.store, row); err != nil {
		return err
	}
	msg := fmt.Sprintf("source created: id=%d name=%q urls=%d", row.ID, row.Name, len(row.URLs))
	h.emit(events.TypeSourceCreated, msg, map[string]any{"id": row.ID})
	h.reload()
	return c.JSON(http.StatusCreated, row)
}

func (h *sourcesHandlers) update(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "store not configured")
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "bad id")
	}
	var in sourceIn
	if err := c.Bind(&in); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	in.URLs = cleanURLs(in.URLs)
	if err := sources.ValidateURLs(in.URLs); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if in.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if in.Weight <= 0 {
		in.Weight = 1
	}
	row := &store.Source{
		ID:      uint(id),
		Name:    in.Name,
		URLs:    in.URLs,
		UA:      in.UA,
		Enabled: in.Enabled,
		Weight:  in.Weight,
	}
	if err := store.SaveSource(h.store, row); err != nil {
		return err
	}
	msg := fmt.Sprintf("source updated: id=%d name=%q urls=%d", row.ID, row.Name, len(row.URLs))
	h.emit(events.TypeSourceUpdated, msg, map[string]any{"id": row.ID})
	h.reload()
	return c.NoContent(http.StatusNoContent)
}

func (h *sourcesHandlers) delete(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "store not configured")
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "bad id")
	}
	if err := store.DeleteSource(h.store, uint(id)); err != nil {
		return err
	}
	msg := fmt.Sprintf("source deleted: id=%d", id)
	h.emit(events.TypeSourceDeleted, msg, map[string]any{"id": id})
	h.reload()
	return c.NoContent(http.StatusNoContent)
}
