package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cyrus/glutton/internal/api"
	"github.com/cyrus/glutton/internal/store"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close(db) })
	return db
}

func TestPutAndGetConfig(t *testing.T) {
	db := newDB(t)
	srv := api.New(api.Deps{Store: db})

	body := `{"daily_quota_gb": 100, "max_rate_mbps": 10, "time_windows": ["* 0-6 * * *"]}`
	req := httptest.NewRequest(http.MethodPut, "/api/config", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	req2 := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)

	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &got))
	require.JSONEq(t, "100", string(got["daily_quota_gb"]))
	require.JSONEq(t, "10", string(got["max_rate_mbps"]))
	require.JSONEq(t, `["* 0-6 * * *"]`, string(got["time_windows"]))
}
