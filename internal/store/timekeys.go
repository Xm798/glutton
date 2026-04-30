package store

import "time"

// HourBucket is the canonical key used in traffic_buckets — Unix seconds at
// the top of the hour containing t.
func HourBucket(t time.Time) int64 {
	return t.Truncate(time.Hour).Unix()
}

// DayStart returns Unix seconds at 00:00 of t's calendar day in loc.
func DayStart(t time.Time, loc *time.Location) int64 {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).Unix()
}

// MonthStart returns Unix seconds at 00:00 on the 1st of t's month in loc.
func MonthStart(t time.Time, loc *time.Location) int64 {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc).Unix()
}
