package api

import (
	"net/http"
	"strconv"

	"github.com/cyrus/glutton/internal/sources"
	"github.com/cyrus/glutton/internal/store"
	"github.com/labstack/echo/v4"
	"gorm.io/gorm"
)

type sourcesHandlers struct {
	store *gorm.DB
}

type sourceIn struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	UA      string `json:"ua"`
	Enabled bool   `json:"enabled"`
	Weight  int    `json:"weight"`
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
	if err := sources.ValidateURL(in.URL); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if in.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if in.Weight <= 0 {
		in.Weight = 1
	}
	row := &store.Source{
		Name: in.Name, URL: in.URL, UA: in.UA,
		Enabled: in.Enabled, Weight: in.Weight,
	}
	if err := store.CreateSource(h.store, row); err != nil {
		return err
	}
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
	if err := sources.ValidateURL(in.URL); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if in.Weight <= 0 {
		in.Weight = 1
	}
	row := &store.Source{
		ID:      uint(id),
		Name:    in.Name,
		URL:     in.URL,
		UA:      in.UA,
		Enabled: in.Enabled,
		Weight:  in.Weight,
	}
	if err := store.SaveSource(h.store, row); err != nil {
		return err
	}
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
	return c.NoContent(http.StatusNoContent)
}
