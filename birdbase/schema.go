package birdbase

import (
	"database/sql"
	"fmt"
)

const schemaVersion = 1

// Specialized tables for different data types
var schema = `
-- Chat conversation history (24h TTL)
CREATE TABLE IF NOT EXISTS chat_history (
    cache_key TEXT PRIMARY KEY,
    messages JSON NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_chat_expires ON chat_history(expires_at);

-- User usage tracking (permanent, no TTL)
CREATE TABLE IF NOT EXISTS user_usage (
    ident TEXT NOT NULL,
    host TEXT NOT NULL,
    total_uses INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (ident, host)
);

-- Command usage leaderboards (permanent, no TTL)
CREATE TABLE IF NOT EXISTS command_leaderboard (
    network TEXT NOT NULL,
    nickname TEXT NOT NULL,
    command TEXT NOT NULL,
    count INTEGER NOT NULL DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (network, nickname, command)
);
CREATE INDEX IF NOT EXISTS idx_leaderboard_network_count ON command_leaderboard(network, count DESC);
CREATE INDEX IF NOT EXISTS idx_leaderboard_command_count ON command_leaderboard(command, count DESC);

-- Persistent network user data (no TTL)
CREATE TABLE IF NOT EXISTS persistent_data (
    data_key TEXT PRIMARY KEY,
    data_type TEXT NOT NULL,
    data_value JSON NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_persistent_type ON persistent_data(data_type);

-- Generic fallback for edge cases
CREATE TABLE IF NOT EXISTS key_value_store (
    key_name TEXT PRIMARY KEY,
    value_data BLOB NOT NULL,
    expires_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_kv_expires ON key_value_store(expires_at);

-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

// initSchema creates the database schema
func (s *SQLiteDB) initSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	var currentVersion int
	err = tx.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if currentVersion < schemaVersion {
		_, err = tx.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
