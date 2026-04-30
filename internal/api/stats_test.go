package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cyrus/glutton/internal/api"
	"github.com/stretchr/testify/require"
)

func TestStatsLive(t *testing.T) {
	db := newDB(t)
	live := &api.LiveCounters{}
	live.Set(123456, 78, 910)

	srv := api.New(api.Deps{Store: db, Live: live})
	req := httptest.NewRequest(http.MethodGet, "/api/stats/live", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.EqualValues(t, 123456, got["current_rate_bps"])
	require.EqualValues(t, 78, got["today_bytes"])
	require.EqualValues(t, 910, got["month_bytes"])
}
