package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cyrus/glutton/internal/api"
	"github.com/stretchr/testify/require"
)

func TestCreateAndListSources(t *testing.T) {
	db := newDB(t)
	srv := api.New(api.Deps{Store: db})

	body := `{"name":"hetzner","url":"https://speed.hetzner.de/100MB.bin","weight":3,"enabled":true}`
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

	body := `{"name":"x","url":"http://10.0.0.1/x","weight":1,"enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/sources", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
