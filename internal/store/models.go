package store

// ConfigKV is a single-row-per-key string store. Values that need structure
// (cron lists, URL lists) are JSON-encoded by the caller before storage.
type ConfigKV struct {
	Key       string `gorm:"primaryKey;column:key"`
	Value     string `gorm:"not null;column:value"`
	UpdatedAt int64  `gorm:"not null;column:updated_at"`
}

func (ConfigKV) TableName() string { return "config_kv" }

type Source struct {
	ID            uint   `gorm:"primaryKey;autoIncrement;column:id"`
	Name          string `gorm:"not null;column:name"`
	URL           string `gorm:"not null;uniqueIndex;column:url"`
	UA            string `gorm:"not null;default:'';column:ua"`
	Enabled       bool   `gorm:"not null;default:true;column:enabled"`
	Weight        int    `gorm:"not null;default:1;column:weight"`
	SuccessCount  int64  `gorm:"not null;default:0;column:success_count"`
	FailCount     int64  `gorm:"not null;default:0;column:fail_count"`
	AvgSpeedBps   int64  `gorm:"not null;default:0;column:avg_speed_bps"`
	LastError     string `gorm:"not null;default:'';column:last_error"`
	LastSuccessAt int64  `gorm:"not null;default:0;column:last_success_at"`
	CooldownUntil int64  `gorm:"not null;default:0;column:cooldown_until"`
	CreatedAt     int64  `gorm:"not null;column:created_at;autoCreateTime:false"`
	UpdatedAt     int64  `gorm:"not null;column:updated_at;autoUpdateTime:false"`
}

func (Source) TableName() string { return "sources" }

// ensureTimestamps stamps CreatedAt (if zero) and UpdatedAt. Called from the
// repo helpers — we don't use GORM's autoCreate/autoUpdate hooks so tests can
// pass explicit times when needed.
func (s *Source) ensureTimestamps(now int64) {
	if s.CreatedAt == 0 {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
}

type TrafficBucket struct {
	HourBucket int64 `gorm:"primaryKey;column:hour_bucket"`
	SourceID   uint  `gorm:"primaryKey;column:source_id"`
	Bytes      int64 `gorm:"not null;column:bytes"`
}

func (TrafficBucket) TableName() string { return "traffic_buckets" }

// Event is the persisted form of events.Event. EventID mirrors the in-process
// monotonic id assigned by the bus so SSE frames and history rows share one
// key space; ID remains the autoincrement DB pk for ordering by insert.
type Event struct {
	ID      uint   `gorm:"primaryKey;autoIncrement;column:id" json:"-"`
	EventID uint64 `gorm:"not null;default:0;column:event_id;index:idx_events_event_id" json:"id"`
	Ts      int64  `gorm:"not null;column:ts;index:idx_events_ts" json:"Ts"`
	Level   string `gorm:"not null;column:level" json:"Level"`
	Type    string `gorm:"not null;column:type" json:"Type"`
	Message string `gorm:"not null;column:message" json:"Message"`
}

func (Event) TableName() string { return "events" }

type SourceTrafficSummary struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
}

func AllModels() []any {
	return []any{&ConfigKV{}, &Source{}, &TrafficBucket{}, &Event{}}
}
