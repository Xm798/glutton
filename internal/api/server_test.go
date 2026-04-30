package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cyrus/glutton/internal/api"
	"github.com/stretchr/testify/require"
)

func TestVersionEndpoint(t *testing.T) {
	srv := api.New(api.Deps{})
	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"version"`)
}

func TestMetricsEndpoint(t *testing.T) {
	srv := api.New(api.Deps{})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "glutton_")
}
