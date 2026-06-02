package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cyrus/glutton/internal/api"
	"github.com/cyrus/glutton/internal/store"
	"github.com/stretchr/testify/require"
)

func TestCreateAndListSources(t *testing.T) {
	db := newDB(t)
	srv := api.New(api.Deps{Store: db})

	body := `{"name":"hetzner","urls":["https://speed.hetzner.de/100MB.bin"],"weight":3,"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	req2 := httptest.NewRequest(http.MethodGet, "/api/sources", nil)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	var out []map[string]any
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &out))
	require.Len(t, out, 1)
	require.Equal(t, "hetzner", out[0]["Name"])
}

func TestCreateSourceRejectsBadURL(t *testing.T) {
	db := newDB(t)
	srv := api.New(api.Deps{Store: db})

	body := `{"name":"x","urls":["http://10.0.0.1/x"],"weight":1,"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestCreateSourceWritesAuditEvent(t *testing.T) {
	db := newDB(t)
	srv := api.New(api.Deps{Store: db})

	body := `{"name":"audit","urls":["https://example.com/100MB.bin"],"weight":1,"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	events, err := store.ListEvents(db, 0, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, "source_created", events[0].Type)
}

func TestCreateSourceRejectsEmptyURLs(t *testing.T) {
	db := newDB(t)
	srv := api.New(api.Deps{Store: db})

	body := `{"name":"x","urls":[],"weight":1,"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

type fakeReloader struct{ n atomic.Int32 }

func (f *fakeReloader) Reload() { f.n.Add(1) }

func TestSourcesReloaderInvokedOnCRUD(t *testing.T) {
	db := newDB(t)
	r := &fakeReloader{}
	srv := api.New(api.Deps{Store: db, SourcesReloader: r})

	body := `{"name":"x","urls":["https://example.com/100MB.bin"],"weight":1,"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	require.Equal(t, int32(1), r.n.Load())

	var created store.Source
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	upd := `{"name":"x2","urls":["https://example.com/100MB.bin"],"weight":2,"enabled":true}`
	req2 := httptest.NewRequest(http.MethodPut, "/api/sources/"+strconv.Itoa(int(created.ID)), strings.NewReader(upd))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusNoContent, rec2.Code, rec2.Body.String())
	require.Equal(t, int32(2), r.n.Load())

	req3 := httptest.NewRequest(http.MethodDelete, "/api/sources/"+strconv.Itoa(int(created.ID)), nil)
	rec3 := httptest.NewRecorder()
	srv.ServeHTTP(rec3, req3)
	require.Equal(t, http.StatusNoContent, rec3.Code)
	require.Equal(t, int32(3), r.n.Load())
}
