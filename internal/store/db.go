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
