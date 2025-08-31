package birdbase

import (
	"aibird/logger"
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	Data              *SQLiteDB
	maintenanceCancel context.CancelFunc
)

type SQLiteDB struct {
	db            *sql.DB
	mu            sync.RWMutex
	cleanupCancel context.CancelFunc
}

// Message represents a chat message (for compatibility)
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Init initializes both SQLite database and in-memory structures
func Init() {
	var err error
	Data, err = NewSQLiteDB("bird.db")
	if err != nil {
		logger.Fatal("Failed to open database", "error", err)
	}

	// Initialize in-memory structures
	InitMemory()

	// Start maintenance loop for database
	ctx, cancel := context.WithCancel(context.Background())
	maintenanceCancel = cancel
	go maintenanceLoop(ctx)
}

// NewSQLiteDB creates a new SQLite database
func NewSQLiteDB(dbPath string) (*SQLiteDB, error) {
	// Open with optimized settings
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=-64000&_temp_store=memory")
	if err != nil {
		return nil, err
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	s := &SQLiteDB{db: db}

	// Initialize schema
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// Get retrieves a value by key
func Get(key string) ([]byte, error) {
	return Data.Get(key)
}

func (s *SQLiteDB) Get(key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var value []byte
	var expiresAt sql.NullTime

	err := s.db.QueryRow(`
        SELECT value_data, expires_at 
        FROM key_value_store 
        WHERE key_name = ? AND (expires_at IS NULL OR expires_at > datetime('now'))
    `, key).Scan(&value, &expiresAt)

	if err != nil {
		return nil, err
	}

	return value, nil
}

// Put stores a value
func Put(key string, value []byte) error {
	return Data.Put(key, value)
}

func (s *SQLiteDB) Put(key string, value []byte) error {
	return s.PutWithTTL(key, value, 0)
}

// PutWithTTL stores a value with TTL
func PutWithTTL(key string, value []byte, ttl time.Duration) error {
	return Data.PutWithTTL(key, value, ttl)
}

func (s *SQLiteDB) PutWithTTL(key string, value []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expiresAt sql.NullTime
	if ttl > 0 {
		expiresAt = sql.NullTime{
			Time:  time.Now().Add(ttl),
			Valid: true,
		}
	}

	_, err := s.db.Exec(`
        INSERT INTO key_value_store (key_name, value_data, expires_at, updated_at)
        VALUES (?, ?, ?, datetime('now'))
        ON CONFLICT(key_name) DO UPDATE SET
            value_data = excluded.value_data,
            expires_at = excluded.expires_at,
            updated_at = datetime('now')
    `, key, value, expiresAt)

	return err
}

// Has checks if a key exists
func Has(key string) bool {
	return Data.Has(key)
}

func (s *SQLiteDB) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var exists bool
	err := s.db.QueryRow(`
        SELECT 1 FROM key_value_store 
        WHERE key_name = ? AND (expires_at IS NULL OR expires_at > datetime('now'))
        LIMIT 1
    `, key).Scan(&exists)

	return err == nil
}

// Delete removes a key
func Delete(key string) error {
	return Data.Delete(key)
}

func (s *SQLiteDB) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM key_value_store WHERE key_name = ?", key)
	return err
}

// Compatibility functions for existing code
func PutString(key string, value string) error {
	return Put(key, []byte(value))
}

func PutInt(key string, value int) error {
	return Put(key, []byte(strconv.Itoa(value)))
}

func PutBytes(key string, value []byte) error {
	return Put(key, value)
}

func PutBytesExpireHours(key string, value []byte, hours int) error {
	return PutWithTTL(key, value, time.Duration(hours)*time.Hour)
}

func PutStringExpireSeconds(key string, value string, seconds int) error {
	return PutWithTTL(key, []byte(value), time.Duration(seconds)*time.Second)
}

func PutIntExpireHours(key string, value int, hours int) error {
	return PutWithTTL(key, []byte(strconv.Itoa(value)), time.Duration(hours)*time.Hour)
}

// Chat History Methods
func PutChatHistory(key string, messages []Message) error {
	return Data.PutChatHistory(key, messages)
}

func (s *SQLiteDB) PutChatHistory(key string, messages []Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
        INSERT INTO chat_history (cache_key, messages, expires_at, updated_at)
        VALUES (?, ?, datetime('now', '+24 hours'), datetime('now'))
        ON CONFLICT(cache_key) DO UPDATE SET
            messages = excluded.messages,
            expires_at = datetime('now', '+24 hours'),
            updated_at = datetime('now')
    `, key, messagesJSON)

	return err
}

func GetChatHistory(key string) ([]Message, error) {
	return Data.GetChatHistory(key)
}

func (s *SQLiteDB) GetChatHistory(key string) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var messagesJSON []byte
	err := s.db.QueryRow(`
        SELECT messages FROM chat_history 
        WHERE cache_key = ? AND expires_at > datetime('now')
    `, key).Scan(&messagesJSON)

	if err != nil {
		return nil, err
	}

	var messages []Message
	err = json.Unmarshal(messagesJSON, &messages)
	return messages, err
}

// User Usage Tracking Methods (with automatic nag detection)
func IncrementUserUsage(ident, host string) (int, bool, error) {
	return Data.IncrementUserUsage(ident, host)
}

func (s *SQLiteDB) IncrementUserUsage(ident, host string) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Increment usage count
	_, err := s.db.Exec(`
        INSERT INTO user_usage (ident, host, total_uses, updated_at)
        VALUES (?, ?, 1, datetime('now'))
        ON CONFLICT(ident, host) DO UPDATE SET
            total_uses = total_uses + 1,
            updated_at = datetime('now')
    `, ident, host)

	if err != nil {
		return 0, false, err
	}

	// Get the new count
	var totalUses int
	err = s.db.QueryRow(`
        SELECT total_uses FROM user_usage WHERE ident = ? AND host = ?
    `, ident, host).Scan(&totalUses)

	if err != nil {
		return 0, false, err
	}

	// Check if should show donation prompt (every 30 uses)
	shouldNag := totalUses > 0 && totalUses%30 == 0

	return totalUses, shouldNag, nil
}

func GetUserUsage(ident, host string) (int, error) {
	return Data.GetUserUsage(ident, host)
}

func (s *SQLiteDB) GetUserUsage(ident, host string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalUses int
	err := s.db.QueryRow(`
        SELECT total_uses FROM user_usage WHERE ident = ? AND host = ?
    `, ident, host).Scan(&totalUses)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	return totalUses, nil
}

// Command Leaderboard Methods
func IncrementCommandUsage(network, nickname, command string) error {
	return Data.IncrementCommandUsage(network, nickname, command)
}

func (s *SQLiteDB) IncrementCommandUsage(network, nickname, command string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
        INSERT INTO command_leaderboard (network, nickname, command, count, updated_at)
        VALUES (?, ?, ?, 1, datetime('now'))
        ON CONFLICT(network, nickname, command) DO UPDATE SET
            count = count + 1,
            updated_at = datetime('now')
    `, network, nickname, command)

	return err
}

func GetNetworkLeaderboard(network string, limit int) ([]LeaderboardEntry, error) {
	return Data.GetNetworkLeaderboard(network, limit)
}

func (s *SQLiteDB) GetNetworkLeaderboard(network string, limit int) ([]LeaderboardEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
        SELECT nickname, command, count, updated_at
        FROM command_leaderboard 
        WHERE network = ?
        ORDER BY count DESC, updated_at DESC
        LIMIT ?
    `, network, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LeaderboardEntry
	for rows.Next() {
		var entry LeaderboardEntry
		err := rows.Scan(&entry.Nickname, &entry.Command, &entry.Count, &entry.UpdatedAt)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func GetGlobalLeaderboard(limit int) ([]GlobalLeaderboardEntry, error) {
	return Data.GetGlobalLeaderboard(limit)
}

func (s *SQLiteDB) GetGlobalLeaderboard(limit int) ([]GlobalLeaderboardEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
        SELECT network, nickname, command, count, updated_at
        FROM command_leaderboard 
        ORDER BY count DESC, updated_at DESC
        LIMIT ?
    `, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []GlobalLeaderboardEntry
	for rows.Next() {
		var entry GlobalLeaderboardEntry
		err := rows.Scan(&entry.Network, &entry.Nickname, &entry.Command, &entry.Count, &entry.UpdatedAt)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

func GetCommandLeaderboard(command string, limit int) ([]GlobalLeaderboardEntry, error) {
	return Data.GetCommandLeaderboard(command, limit)
}

func (s *SQLiteDB) GetCommandLeaderboard(command string, limit int) ([]GlobalLeaderboardEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
        SELECT network, nickname, command, count, updated_at
        FROM command_leaderboard 
        WHERE command = ?
        ORDER BY count DESC, updated_at DESC
        LIMIT ?
    `, command, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []GlobalLeaderboardEntry
	for rows.Next() {
		var entry GlobalLeaderboardEntry
		err := rows.Scan(&entry.Network, &entry.Nickname, &entry.Command, &entry.Count, &entry.UpdatedAt)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// Leaderboard data structures
type LeaderboardEntry struct {
	Nickname  string    `json:"nickname"`
	Command   string    `json:"command"`
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GlobalLeaderboardEntry struct {
	Network   string    `json:"network"`
	Nickname  string    `json:"nickname"`
	Command   string    `json:"command"`
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Persistent Data Methods
func PutPersistentData(key, dataType string, value interface{}) error {
	return Data.PutPersistentData(key, dataType, value)
}

func (s *SQLiteDB) PutPersistentData(key, dataType string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	valueJSON, err := json.Marshal(value)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
        INSERT INTO persistent_data (data_key, data_type, data_value, updated_at)
        VALUES (?, ?, ?, datetime('now'))
        ON CONFLICT(data_key) DO UPDATE SET
            data_type = excluded.data_type,
            data_value = excluded.data_value,
            updated_at = datetime('now')
    `, key, dataType, valueJSON)

	return err
}

func GetPersistentData(key string, result interface{}) error {
	return Data.GetPersistentData(key, result)
}

func (s *SQLiteDB) GetPersistentData(key string, result interface{}) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var valueJSON []byte
	err := s.db.QueryRow(`
        SELECT data_value FROM persistent_data WHERE data_key = ?
    `, key).Scan(&valueJSON)

	if err != nil {
		return err
	}

	return json.Unmarshal(valueJSON, result)
}

// Stats returns database statistics
func GetDatabaseStats() (map[string]any, error) {
	return Data.Stats()
}

func (s *SQLiteDB) Stats() (map[string]any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalKeys, sizeBytes int64

	// Count total keys
	err := s.db.QueryRow("SELECT COUNT(*) FROM key_value_store WHERE expires_at IS NULL OR expires_at > datetime('now')").Scan(&totalKeys)
	if err != nil {
		return nil, err
	}

	// Get approximate database size
	err = s.db.QueryRow("SELECT page_count * page_size as size FROM pragma_page_count(), pragma_page_size()").Scan(&sizeBytes)
	if err != nil {
		sizeBytes = 0 // Not critical if this fails
	}

	return map[string]any{
		"keys": totalKeys,
		"size": sizeBytes,
	}, nil
}

// Close closes the database
func Close() {
	logger.Info("Closing database...")

	// Cancel maintenance loop
	if maintenanceCancel != nil {
		maintenanceCancel()
	}

	// Final cleanup
	if Data != nil {
		Data.Cleanup()
		Data.db.Close()
		logger.Info("Database closed successfully")
	}
}

// Merge is kept for compatibility (does nothing since SQLite doesn't need merging)
func Merge() {
	logger.Info("Merge operation not needed for SQLite")
}