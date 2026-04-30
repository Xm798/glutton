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
		"name", "url", "ua", "enabled", "weight", "updated_at",
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

func InsertEvent(db *gorm.DB, e *Event) error {
	return db.Create(e).Error
}

func ListEvents(db *gorm.DB, since int64, limit int) ([]Event, error) {
	var rows []Event
	err := db.Where("ts >= ?", since).Order("ts DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func PurgeEventsBefore(db *gorm.DB, ts int64) error {
	return db.Where("ts < ?", ts).Delete(&Event{}).Error
}
