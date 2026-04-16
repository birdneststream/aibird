package birdbase

import (
	"database/sql"
	"fmt"
)

const schemaVersion = 3

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

-- Generic fallback for edge cases
CREATE TABLE IF NOT EXISTS key_value_store (
    key_name TEXT PRIMARY KEY,
    value_data BLOB NOT NULL,
    expires_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_kv_expires ON key_value_store(expires_at);

-- ===========================================
-- NORMALIZED IRC DATA SCHEMA (Version 3)
-- ===========================================

-- Networks table - core network configurations
CREATE TABLE IF NOT EXISTS networks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    network_name TEXT NOT NULL UNIQUE,
    enabled BOOLEAN NOT NULL DEFAULT 1,
    nick TEXT NOT NULL,
    user_name TEXT,
    real_name TEXT,
    preserve_modes BOOLEAN NOT NULL DEFAULT 0,
    ping_delay INTEGER DEFAULT 30,
    version TEXT,
    throttle INTEGER DEFAULT 300,
    burst INTEGER DEFAULT 5,
    action_trigger TEXT DEFAULT '!',
    modes_at_once INTEGER DEFAULT 4,
    nickserv_pass TEXT, -- Store encrypted passwords
    server_pass TEXT,   -- Store encrypted passwords
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Channels table - channel configurations per network
CREATE TABLE IF NOT EXISTS channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    preserve_modes BOOLEAN NOT NULL DEFAULT 0,
    ai_enabled BOOLEAN NOT NULL DEFAULT 0,
    sd_enabled BOOLEAN NOT NULL DEFAULT 0,
    image_describe BOOLEAN NOT NULL DEFAULT 0,
    sound_enabled BOOLEAN NOT NULL DEFAULT 0,
    video_enabled BOOLEAN NOT NULL DEFAULT 0,
    action_trigger TEXT,
    trim_output BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (network_id) REFERENCES networks(id) ON DELETE CASCADE,
    UNIQUE(network_id, name)
);

-- IRC Users table - normalized user data
CREATE TABLE IF NOT EXISTS irc_users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id INTEGER NOT NULL,
    nickname TEXT NOT NULL,
    ident TEXT NOT NULL,
    host TEXT NOT NULL,
    first_seen INTEGER NOT NULL, -- Unix timestamp
    latest_activity INTEGER DEFAULT 0, -- Unix timestamp  
    latest_chat TEXT,
    is_admin BOOLEAN NOT NULL DEFAULT 0,
    is_owner BOOLEAN NOT NULL DEFAULT 0,
    ignored BOOLEAN NOT NULL DEFAULT 0,
    access_level INTEGER NOT NULL DEFAULT 0,
    ai_service TEXT DEFAULT 'llamacpp',
    ai_model TEXT,
    ai_base_prompt TEXT,
    ai_personality TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (network_id) REFERENCES networks(id) ON DELETE CASCADE,
    UNIQUE(network_id, ident, host) -- Prevent duplicate users per network
);

-- User modes table - normalized mode storage
CREATE TABLE IF NOT EXISTS user_modes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    channel_id INTEGER NOT NULL,
    mode_type TEXT NOT NULL CHECK (mode_type IN ('preserved', 'current')),
    modes TEXT NOT NULL, -- JSON array of mode strings for flexibility
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES irc_users(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
    UNIQUE(user_id, channel_id, mode_type) -- One preserved and one current mode set per user per channel
);

-- User-Channel associations (many-to-many)
CREATE TABLE IF NOT EXISTS user_channels (
    user_id INTEGER NOT NULL,
    channel_id INTEGER NOT NULL,
    joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, channel_id),
    FOREIGN KEY (user_id) REFERENCES irc_users(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE
);

-- Servers table - normalized server configurations
CREATE TABLE IF NOT EXISTS servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id INTEGER NOT NULL,
    host TEXT NOT NULL,
    port INTEGER NOT NULL DEFAULT 6667,
    ssl BOOLEAN NOT NULL DEFAULT 0,
    skip_ssl_verify BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (network_id) REFERENCES networks(id) ON DELETE CASCADE,
    UNIQUE(network_id, host, port)
);

-- Admin hosts table - normalized admin permissions
CREATE TABLE IF NOT EXISTS admin_hosts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id INTEGER NOT NULL,
    host TEXT NOT NULL,
    ident TEXT NOT NULL,
    is_owner BOOLEAN NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (network_id) REFERENCES networks(id) ON DELETE CASCADE,
    UNIQUE(network_id, host, ident)
);

-- Ignored nicks table - normalized ignore list
CREATE TABLE IF NOT EXISTS ignored_nicks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id INTEGER NOT NULL,
    nickname TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (network_id) REFERENCES networks(id) ON DELETE CASCADE,
    UNIQUE(network_id, nickname)
);

-- Denied commands table - normalized command restrictions
CREATE TABLE IF NOT EXISTS denied_commands (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    network_id INTEGER,
    channel_id INTEGER,
    command TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (network_id) REFERENCES networks(id) ON DELETE CASCADE,
    FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE CASCADE,
    CHECK ((network_id IS NOT NULL AND channel_id IS NULL) OR 
           (network_id IS NULL AND channel_id IS NOT NULL)), -- Either network-level or channel-level
    UNIQUE(network_id, channel_id, command) -- Prevent duplicate commands per network/channel
);

-- Performance indexes for normalized schema
CREATE INDEX IF NOT EXISTS idx_irc_users_network_ident_host ON irc_users(network_id, ident, host);
CREATE INDEX IF NOT EXISTS idx_irc_users_nickname ON irc_users(nickname);
CREATE INDEX IF NOT EXISTS idx_irc_users_access_level ON irc_users(access_level);
CREATE INDEX IF NOT EXISTS idx_irc_users_activity ON irc_users(latest_activity);
CREATE INDEX IF NOT EXISTS idx_user_modes_user_channel ON user_modes(user_id, channel_id);
CREATE INDEX IF NOT EXISTS idx_user_modes_type ON user_modes(mode_type);
CREATE INDEX IF NOT EXISTS idx_channels_network_name ON channels(network_id, name);
CREATE INDEX IF NOT EXISTS idx_servers_network ON servers(network_id);
CREATE INDEX IF NOT EXISTS idx_admin_hosts_network ON admin_hosts(network_id);
CREATE INDEX IF NOT EXISTS idx_ignored_nicks_network ON ignored_nicks(network_id);

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

	if _, execErr := tx.Exec(schema); execErr != nil {
		return fmt.Errorf("failed to create schema: %w", execErr)
	}

	var currentVersion int
	err = tx.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&currentVersion)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if currentVersion < schemaVersion {
		// Migration 1→2: Replace ollama service references with llamacpp
		if currentVersion < 2 {
			if _, migErr := tx.Exec("UPDATE irc_users SET ai_service = 'llamacpp' WHERE ai_service = 'ollama'"); migErr != nil {
				return fmt.Errorf("failed to migrate ai_service from ollama to llamacpp: %w", migErr)
			}
		}

		// Migration 2→3: Replace openrouter service references with llamacpp
		if currentVersion < 3 {
			if _, migErr := tx.Exec("UPDATE irc_users SET ai_service = 'llamacpp' WHERE ai_service = 'openrouter'"); migErr != nil {
				return fmt.Errorf("failed to migrate ai_service from openrouter to llamacpp: %w", migErr)
			}
		}

		_, err = tx.Exec("INSERT INTO schema_version (version) VALUES (?)", schemaVersion)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}
