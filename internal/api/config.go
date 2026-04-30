package api

import (
	"encoding/json"
	"net/http"

	"github.com/cyrus/glutton/internal/store"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type configHandlers struct {
	store *gorm.DB
}

func (h *configHandlers) get(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "store not configured")
	}
	rows, err := store.ListConfig(h.store)
	if err != nil {
		return err
	}
	out := make(map[string]json.RawMessage, len(rows))
	for k, v := range rows {
		out[k] = json.RawMessage(v)
	}
	return c.JSON(http.StatusOK, out)
}

func (h *configHandlers) put(c echo.Context) error {
	if h.store == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "store not configured")
	}
	var in map[string]json.RawMessage
	if err := json.NewDecoder(c.Request().Body).Decode(&in); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid json body")
	}
	for k, v := range in {
		if err := store.UpsertConfig(h.store, k, string(v)); err != nil {
			return err
		}
	}
	return c.NoContent(http.StatusNoContent)
}
