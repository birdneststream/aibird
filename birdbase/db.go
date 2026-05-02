package birdbase

import (
	"context"
	"database/sql"
	"sync"
	"time"

	"aibird/logger"

	_ "github.com/mattn/go-sqlite3"
)

var (
	Data              *SQLiteDB
	maintenanceCancel context.CancelFunc
)

type SQLiteDB struct {
	db *sql.DB
	mu sync.RWMutex
}

func Init() {
	var err error
	Data, err = NewSQLiteDB("bird.db")
	if err != nil {
		logger.Fatal("Failed to open database", "error", err)
	}

	InitMemory()

	ctx, cancel := context.WithCancel(context.Background())
	maintenanceCancel = cancel
	go maintenanceLoop(ctx)
}

func NewSQLiteDB(dbPath string) (*SQLiteDB, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=-64000&_temp_store=memory&_foreign_keys=1&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	s := &SQLiteDB{db: db}

	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func GetDatabaseStats() (map[string]any, error) {
	return Data.Stats()
}

func (s *SQLiteDB) Stats() (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalKeys, sizeBytes int64

	err := s.db.QueryRow("SELECT COUNT(*) FROM key_value_store WHERE expires_at IS NULL OR expires_at > datetime('now')").Scan(&totalKeys)
	if err != nil {
		return nil, err
	}

	err = s.db.QueryRow("SELECT page_count * page_size as size FROM pragma_page_count(), pragma_page_size()").Scan(&sizeBytes)
	if err != nil {
		sizeBytes = 0
	}

	return map[string]any{
		"keys": totalKeys,
		"size": sizeBytes,
	}, nil
}

func Close() {
	logger.Info("Closing database...")

	if maintenanceCancel != nil {
		maintenanceCancel()
	}

	StopMemory()

	if Data != nil {
		Data.Cleanup()
		Data.db.Close()
		logger.Info("Database closed successfully")
	}
}
