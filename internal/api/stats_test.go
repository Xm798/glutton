package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cyrus/glutton/internal/api"
	"github.com/cyrus/glutton/internal/store"
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

func TestStatsSeriesMinuteRange(t *testing.T) {
	db := newDB(t)
	now := time.Now()
	require.NoError(t, store.SetMinuteSample(db, store.MinuteBucket(now.Add(-30*time.Minute)), 1000))
	require.NoError(t, store.SetMinuteSample(db, store.MinuteBucket(now.Add(-2*time.Minute)), 2000))

	srv := api.New(api.Deps{Store: db})
	req := httptest.NewRequest(http.MethodGet, "/api/stats/series?range=1h", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		Step   int64 `json:"step"`
		Points []struct {
			T     int64 `json:"t"`
			Bytes int64 `json:"bytes"`
		} `json:"points"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, int64(60), got.Step)
	require.NotEmpty(t, got.Points)
	var total int64
	zero := 0
	for _, p := range got.Points {
		total += p.Bytes
		if p.Bytes == 0 {
			zero++
		}
	}
	require.Equal(t, int64(3000), total)
	require.Greater(t, zero, 0)
}

func TestStatsSeriesHourlyRange(t *testing.T) {
	db := newDB(t)
	s := &store.Source{Name: "x", URLs: []string{"https://example.com/x"}, Weight: 1, Enabled: true}
	require.NoError(t, store.CreateSource(db, s))
	now := time.Now()
	require.NoError(t, store.AddTraffic(db, store.HourBucket(now.Add(-2*time.Hour)), s.ID, 5000))

	srv := api.New(api.Deps{Store: db})
	req := httptest.NewRequest(http.MethodGet, "/api/stats/series?range=1w", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		Step   int64 `json:"step"`
		Points []struct{ T, Bytes int64 }
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, int64(3600), got.Step)
	var total int64
	for _, p := range got.Points {
		total += p.Bytes
	}
	require.Equal(t, int64(5000), total)
}

func TestStatsSeriesDefaultsToDay(t *testing.T) {
	db := newDB(t)
	srv := api.New(api.Deps{Store: db})
	req := httptest.NewRequest(http.MethodGet, "/api/stats/series?range=bogus", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		Step int64 `json:"step"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, int64(600), got.Step)
}
