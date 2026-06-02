package store

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Open(dsn string) (*gorm.DB, error) {
	if dsn == ":memory:" {
		dsn = "file::memory:?cache=shared"
	}
	gormLogger := logger.New(
		log.New(os.Stderr, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	// One-time migration: the multi-URL source model replaces the single `url`
	// column with a json `urls` list. Old data is intentionally discarded — if a
	// legacy `url` column is present, drop sources (sheds the old column + unique
	// index) and traffic_buckets (its source_id rows would reference stale ids).
	if legacySourcesPresent(db) {
		if err := db.Migrator().DropTable("sources"); err != nil {
			return nil, fmt.Errorf("drop legacy sources: %w", err)
		}
		if err := db.Migrator().DropTable("traffic_buckets"); err != nil {
			return nil, fmt.Errorf("drop legacy traffic_buckets: %w", err)
		}
	}
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return nil, fmt.Errorf("automigrate: %w", err)
	}
	// WAL mode for concurrent reads; NORMAL sync is safe with WAL and improves throughput.
	// :memory: databases silently ignore both PRAGMAs, so tests are unaffected.
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		return nil, fmt.Errorf("enable WAL: %w", err)
	}
	if err := db.Exec("PRAGMA synchronous=NORMAL").Error; err != nil {
		return nil, fmt.Errorf("set synchronous: %w", err)
	}
	return db, nil
}

func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// legacySourcesPresent reports whether a pre-multi-URL sources table (with the
// old `url` column) exists. Uses sqlite's pragma_table_info so it's independent
// of the current Go struct shape — checking HasColumn(&Source{}, "url") would
// parse the new struct and always miss.
func legacySourcesPresent(db *gorm.DB) bool {
	if !db.Migrator().HasTable("sources") {
		return false
	}
	var n int64
	if err := db.Raw(
		`SELECT COUNT(*) FROM pragma_table_info('sources') WHERE name = 'url'`,
	).Scan(&n).Error; err != nil {
		return false
	}
	return n > 0
}
