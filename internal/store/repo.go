package store

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func UpsertConfig(db *gorm.DB, key, value string) error {
	row := ConfigKV{Key: key, Value: value, UpdatedAt: time.Now().Unix()}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
}

func GetConfig(db *gorm.DB, key string) (string, error) {
	var row ConfigKV
	if err := db.First(&row, "key = ?", key).Error; err != nil {
		return "", err
	}
	return row.Value, nil
}

func ListConfig(db *gorm.DB) (map[string]string, error) {
	var rows []ConfigKV
	if err := db.Order("key").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.Key] = r.Value
	}
	return out, nil
}

func CreateSource(db *gorm.DB, s *Source) error {
	s.ensureTimestamps(time.Now().Unix())
	return db.Create(s).Error
}

// SaveSource updates user-editable fields on an existing row. Health-stat
// columns (success_count, fail_count, avg_speed_bps, last_error,
// last_success_at, cooldown_until) are NOT touched here — those are owned
// by RecordSourceSuccess/RecordSourceFailure to avoid races with the consumer.
func SaveSource(db *gorm.DB, s *Source) error {
	s.UpdatedAt = time.Now().Unix()
	return db.Model(s).Select(
		"name", "urls", "ua", "enabled", "weight", "updated_at",
	).Updates(s).Error
}

func DeleteSource(db *gorm.DB, id uint) error {
	return db.Delete(&Source{}, id).Error
}

func ListSources(db *gorm.DB) ([]Source, error) {
	var rows []Source
	err := db.Order("id").Find(&rows).Error
	return rows, err
}

func ListEnabledSources(db *gorm.DB) ([]Source, error) {
	var rows []Source
	err := db.Where("enabled = ?", true).Order("id").Find(&rows).Error
	return rows, err
}

func RecordSourceSuccess(db *gorm.DB, id uint, avgSpeedBps int64, ts int64) error {
	return db.Model(&Source{}).Where("id = ?", id).Updates(map[string]any{
		"success_count":   gorm.Expr("success_count + 1"),
		"avg_speed_bps":   avgSpeedBps,
		"last_success_at": ts,
		"last_error":      "",
		"cooldown_until":  0,
		"updated_at":      time.Now().Unix(),
	}).Error
}

func RecordSourceFailure(db *gorm.DB, id uint, errMsg string, cooldownUntil int64) error {
	return db.Model(&Source{}).Where("id = ?", id).Updates(map[string]any{
		"fail_count":     gorm.Expr("fail_count + 1"),
		"last_error":     errMsg,
		"cooldown_until": cooldownUntil,
		"updated_at":     time.Now().Unix(),
	}).Error
}

func AddTraffic(db *gorm.DB, hourBucket int64, sourceID uint, bytes int64) error {
	row := TrafficBucket{HourBucket: hourBucket, SourceID: sourceID, Bytes: bytes}
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "hour_bucket"}, {Name: "source_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"bytes": gorm.Expr("traffic_buckets.bytes + ?", bytes),
		}),
	}).Create(&row).Error
}

// SumTrafficSince returns the total bytes across all sources for hour buckets
// >= since. Used at startup to recover daily/monthly counters from the
// persistent traffic_buckets table after a restart.
func SumTrafficSince(db *gorm.DB, since int64) (int64, error) {
	var sum int64
	err := db.Model(&TrafficBucket{}).
		Where("hour_bucket >= ?", since).
		Select("COALESCE(SUM(bytes), 0)").
		Scan(&sum).Error
	return sum, err
}

func TrafficSinceBucket(db *gorm.DB, since int64) ([]TrafficBucket, error) {
	var rows []TrafficBucket
	err := db.Where("hour_bucket >= ?", since).Order("hour_bucket").Find(&rows).Error
	return rows, err
}

func TrafficBySource(db *gorm.DB, since int64) ([]SourceTrafficSummary, error) {
	var rows []SourceTrafficSummary
	err := db.Table("sources s").
		Select("s.id, s.name, COALESCE(SUM(tb.bytes), 0) AS bytes").
		Joins("LEFT JOIN traffic_buckets tb ON tb.source_id = s.id AND tb.hour_bucket >= ?", since).
		Group("s.id").
		Order("bytes DESC").
		Scan(&rows).Error
	return rows, err
}

func PurgeTrafficBefore(db *gorm.DB, hourBucket int64) error {
	return db.Where("hour_bucket < ?", hourBucket).Delete(&TrafficBucket{}).Error
}

// SetMinuteSample writes the total bytes for a minute bucket, replacing any
// prior value. The sampler refreshes the in-progress minute on every tick so
// the high-resolution chart reflects live traffic, not just completed minutes;
// in-minute accumulation is held in memory, so the stored value is absolute.
func SetMinuteSample(db *gorm.DB, minuteBucket, bytes int64) error {
	row := MinuteSample{MinuteBucket: minuteBucket, Bytes: bytes}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "minute_bucket"}},
		DoUpdates: clause.Assignments(map[string]any{"bytes": bytes}),
	}).Create(&row).Error
}

func MinuteSamplesSince(db *gorm.DB, since int64) ([]MinuteSample, error) {
	var rows []MinuteSample
	err := db.Where("minute_bucket >= ?", since).Order("minute_bucket").Find(&rows).Error
	return rows, err
}

func PurgeMinuteSamplesBefore(db *gorm.DB, minuteBucket int64) error {
	return db.Where("minute_bucket < ?", minuteBucket).Delete(&MinuteSample{}).Error
}

func InsertEvent(db *gorm.DB, e *Event) error {
	return db.Create(e).Error
}

func ListEvents(db *gorm.DB, since int64, limit int) ([]Event, error) {
	return ListEventsFiltered(db, since, limit, nil)
}

// ListEventsFiltered returns the most-recent `limit` events with ts >= since.
// When types is non-empty the predicate is pushed into SQL (`type IN (...)`)
// so the limit is applied AFTER the filter, not before — otherwise a
// type-specific request can return zero rows even when matching rows exist
// just past the global limit window.
func ListEventsFiltered(db *gorm.DB, since int64, limit int, types []string) ([]Event, error) {
	var rows []Event
	q := db.Where("ts >= ?", since)
	if len(types) > 0 {
		cleaned := make([]string, 0, len(types))
		for _, t := range types {
			if t != "" {
				cleaned = append(cleaned, t)
			}
		}
		if len(cleaned) > 0 {
			q = q.Where("type IN ?", cleaned)
		}
	}
	err := q.Order("ts DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func PurgeEventsBefore(db *gorm.DB, ts int64) error {
	return db.Where("ts < ?", ts).Delete(&Event{}).Error
}

// MaxEventID returns the highest event_id ever persisted, or 0 if the table
// is empty. Used at startup to seed the in-process bus monotonic counter so
// SSE/history ids never collide with rows from the previous run.
func MaxEventID(db *gorm.DB) (uint64, error) {
	var max uint64
	err := db.Model(&Event{}).Select("COALESCE(MAX(event_id), 0)").Scan(&max).Error
	return max, err
}
