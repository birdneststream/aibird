package birdbase

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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
func PutString(key, value string) error {
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

func PutStringExpireSeconds(key, value string, seconds int) error {
	return PutWithTTL(key, []byte(value), time.Duration(seconds)*time.Second)
}

func PutIntExpireHours(key string, value, hours int) error {
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

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func GetNetworkUserTotals(network string, limit int) ([]UserTotalEntry, error) {
	return Data.GetNetworkUserTotals(network, limit)
}

func (s *SQLiteDB) GetNetworkUserTotals(network string, limit int) ([]UserTotalEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
        SELECT nickname, SUM(count) as total_count, MAX(updated_at) as latest_update
        FROM command_leaderboard 
        WHERE network = ?
        GROUP BY nickname
        ORDER BY total_count DESC, latest_update DESC
        LIMIT ?
    `, network, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []UserTotalEntry
	for rows.Next() {
		var entry UserTotalEntry
		err := rows.Scan(&entry.Nickname, &entry.TotalCount, &entry.UpdatedAt)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, err
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

	if err := rows.Err(); err != nil {
		return nil, err
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

	if err := rows.Err(); err != nil {
		return nil, err
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

type UserTotalEntry struct {
	Nickname   string    `json:"nickname"`
	TotalCount int       `json:"total_count"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// REMOVED: Legacy JSON blob storage methods
// All data now uses normalized database schema above

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

// ===========================================
// NORMALIZED IRC DATA METHODS
// ===========================================

// Network Methods
func SaveNetwork(networkName string, network *NetworkData) error {
	return Data.SaveNetwork(networkName, network)
}

type NetworkData struct {
	Enabled       bool
	Nick          string
	User          string
	Name          string
	Pass          string
	PreserveModes bool
	NickServPass  string
	PingDelay     int
	Version       string
	Throttle      int
	Burst         int
	ActionTrigger string
	ModesAtOnce   int
	IgnoredNicks  []string
	DenyCommands  []string
	AdminHosts    []AdminHost
	Servers       []ServerData
	Channels      []ChannelData
}

type AdminHost struct {
	Host  string
	Ident string
	Owner bool
}

type ServerData struct {
	Host          string
	Port          int
	SSL           bool
	SkipSSLVerify bool
}

type ChannelData struct {
	Name          string
	PreserveModes bool
	Ai            bool
	Sd            bool
	ImageDescribe bool
	Sound         bool
	Video         bool
	ActionTrigger string
	TrimOutput    bool
	DenyCommands  []string
}

type UserData struct {
	ID             int
	NetworkID      int
	NickName       string
	Ident          string
	Host           string
	FirstSeen      int64
	LatestActivity int64
	LatestChat     string
	IsAdmin        bool
	IsOwner        bool
	Ignored        bool
	AccessLevel    int
	AiService      string
	AiModel        string
	AiBasePrompt   string
	AiPersonality  string
	PreservedModes []UserModeData
	CurrentModes   []UserModeData
}

type UserModeData struct {
	Channel string
	Modes   []string
}

func (s *SQLiteDB) SaveNetwork(networkName string, network *NetworkData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert or update network
	networkID := int64(0)
	err = tx.QueryRow(`
		INSERT INTO networks (network_name, enabled, nick, user_name, real_name, preserve_modes, 
			ping_delay, version, throttle, burst, action_trigger, modes_at_once, 
			nickserv_pass, server_pass, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(network_name) DO UPDATE SET
			enabled = excluded.enabled,
			nick = excluded.nick,
			user_name = excluded.user_name,
			real_name = excluded.real_name,
			preserve_modes = excluded.preserve_modes,
			ping_delay = excluded.ping_delay,
			version = excluded.version,
			throttle = excluded.throttle,
			burst = excluded.burst,
			action_trigger = excluded.action_trigger,
			modes_at_once = excluded.modes_at_once,
			nickserv_pass = excluded.nickserv_pass,
			server_pass = excluded.server_pass,
			updated_at = datetime('now')
		RETURNING id
	`, networkName, network.Enabled, network.Nick, network.User, network.Name, network.PreserveModes,
		network.PingDelay, network.Version, network.Throttle, network.Burst, network.ActionTrigger,
		network.ModesAtOnce, network.NickServPass, network.Pass).Scan(&networkID)

	if err != nil {
		return err
	}

	// Sync servers: upsert current config, delete orphaned ones
	if len(network.Servers) > 0 {
		// Upsert servers from config using INSERT ... ON CONFLICT to preserve IDs
		for _, server := range network.Servers {
			_, err = tx.Exec(`
				INSERT INTO servers (network_id, host, port, ssl, skip_ssl_verify)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(network_id, host, port) DO UPDATE SET
					ssl = excluded.ssl,
					skip_ssl_verify = excluded.skip_ssl_verify
			`, networkID, server.Host, server.Port, server.SSL, server.SkipSSLVerify)
			if err != nil {
				return err
			}
		}

		// Build list of servers that should exist
		var serverHosts []string
		for _, server := range network.Servers {
			serverHosts = append(serverHosts, server.Host)
		}

		// Delete servers not in current config
		placeholders := make([]string, len(serverHosts))
		args := make([]interface{}, len(serverHosts)+1)
		args[0] = networkID
		for i, host := range serverHosts {
			placeholders[i] = "?"
			args[i+1] = host
		}

		deleteQuery := fmt.Sprintf(`
			DELETE FROM servers 
			WHERE network_id = ? AND host NOT IN (%s)
		`, strings.Join(placeholders, ","))

		_, err = tx.Exec(deleteQuery, args...)
		if err != nil {
			return err
		}
	} else {
		// No servers in config, delete all for this network
		_, err = tx.Exec("DELETE FROM servers WHERE network_id = ?", networkID)
		if err != nil {
			return err
		}
	}

	// Sync admin hosts: upsert current config, delete orphaned ones
	if len(network.AdminHosts) > 0 {
		// Upsert admin hosts from config using INSERT ... ON CONFLICT to preserve IDs
		for _, admin := range network.AdminHosts {
			_, err = tx.Exec(`
				INSERT INTO admin_hosts (network_id, host, ident, is_owner)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(network_id, host, ident) DO UPDATE SET
					is_owner = excluded.is_owner
			`, networkID, admin.Host, admin.Ident, admin.Owner)
			if err != nil {
				return err
			}
		}

		// Delete admin hosts not in current config (host+ident pairs)
		var hostIdentPairs []string
		var args []interface{}
		args = append(args, networkID)

		for _, admin := range network.AdminHosts {
			hostIdentPairs = append(hostIdentPairs, "(host = ? AND ident = ?)")
			args = append(args, admin.Host, admin.Ident)
		}

		deleteQuery := fmt.Sprintf(`
			DELETE FROM admin_hosts 
			WHERE network_id = ? AND NOT (%s)
		`, strings.Join(hostIdentPairs, " OR "))

		_, err = tx.Exec(deleteQuery, args...)
		if err != nil {
			return err
		}
	} else {
		// No admin hosts in config, delete all for this network
		_, err = tx.Exec("DELETE FROM admin_hosts WHERE network_id = ?", networkID)
		if err != nil {
			return err
		}
	}

	// Sync channels: upsert current config, delete orphaned ones
	if len(network.Channels) > 0 {
		// Upsert channels from config using INSERT ... ON CONFLICT to preserve IDs
		for _, channel := range network.Channels {
			_, err = tx.Exec(`
				INSERT INTO channels (network_id, name, preserve_modes, ai_enabled, sd_enabled, 
					image_describe, sound_enabled, video_enabled, action_trigger, trim_output)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(network_id, name) DO UPDATE SET
					preserve_modes = excluded.preserve_modes,
					ai_enabled = excluded.ai_enabled,
					sd_enabled = excluded.sd_enabled,
					image_describe = excluded.image_describe,
					sound_enabled = excluded.sound_enabled,
					video_enabled = excluded.video_enabled,
					action_trigger = excluded.action_trigger,
					trim_output = excluded.trim_output,
					updated_at = datetime('now')
			`, networkID, channel.Name, channel.PreserveModes, channel.Ai, channel.Sd,
				channel.ImageDescribe, channel.Sound, channel.Video, channel.ActionTrigger, channel.TrimOutput)
			if err != nil {
				return err
			}

			// Handle channel-level denied commands
			channelID := int64(0)
			err = tx.QueryRow("SELECT id FROM channels WHERE network_id = ? AND name = ?", networkID, channel.Name).Scan(&channelID)
			if err != nil {
				return err
			}

			// Sync channel denied commands
			if len(channel.DenyCommands) > 0 {
				for _, cmd := range channel.DenyCommands {
					_, err = tx.Exec(`
						INSERT INTO denied_commands (network_id, channel_id, command)
						VALUES (NULL, ?, ?)
						ON CONFLICT(network_id, channel_id, command) DO NOTHING
					`, channelID, cmd)
					if err != nil {
						return err
					}
				}

				// Delete channel denied commands not in current config
				placeholders := make([]string, len(channel.DenyCommands))
				args := make([]interface{}, len(channel.DenyCommands)+1)
				args[0] = channelID
				for i, cmd := range channel.DenyCommands {
					placeholders[i] = "?"
					args[i+1] = cmd
				}

				deleteQuery := fmt.Sprintf(`
					DELETE FROM denied_commands 
					WHERE channel_id = ? AND command NOT IN (%s)
				`, strings.Join(placeholders, ","))

				_, err = tx.Exec(deleteQuery, args...)
				if err != nil {
					return err
				}
			} else {
				// No channel denied commands in config, delete all for this channel
				_, err = tx.Exec("DELETE FROM denied_commands WHERE channel_id = ?", channelID)
				if err != nil {
					return err
				}
			}
		}

		// Delete channels not in current config
		placeholders := make([]string, len(network.Channels))
		args := make([]interface{}, len(network.Channels)+1)
		args[0] = networkID
		for i, channel := range network.Channels {
			placeholders[i] = "?"
			args[i+1] = channel.Name
		}

		deleteQuery := fmt.Sprintf(`
			DELETE FROM channels 
			WHERE network_id = ? AND name NOT IN (%s)
		`, strings.Join(placeholders, ","))

		_, err = tx.Exec(deleteQuery, args...)
		if err != nil {
			return err
		}
	} else {
		// No channels in config, delete all for this network
		_, err = tx.Exec("DELETE FROM channels WHERE network_id = ?", networkID)
		if err != nil {
			return err
		}
	}

	// Sync ignored nicks: upsert current config, delete orphaned ones
	if len(network.IgnoredNicks) > 0 {
		// Upsert ignored nicks from config using INSERT ... ON CONFLICT to preserve IDs
		for _, nick := range network.IgnoredNicks {
			_, err = tx.Exec(`
				INSERT INTO ignored_nicks (network_id, nickname)
				VALUES (?, ?)
				ON CONFLICT(network_id, nickname) DO NOTHING
			`, networkID, nick)
			if err != nil {
				return err
			}
		}

		// Delete ignored nicks not in current config
		placeholders := make([]string, len(network.IgnoredNicks))
		args := make([]interface{}, len(network.IgnoredNicks)+1)
		args[0] = networkID
		for i, nick := range network.IgnoredNicks {
			placeholders[i] = "?"
			args[i+1] = nick
		}

		deleteQuery := fmt.Sprintf(`
			DELETE FROM ignored_nicks 
			WHERE network_id = ? AND nickname NOT IN (%s)
		`, strings.Join(placeholders, ","))

		_, err = tx.Exec(deleteQuery, args...)
		if err != nil {
			return err
		}
	} else {
		// No ignored nicks in config, delete all for this network
		_, err = tx.Exec("DELETE FROM ignored_nicks WHERE network_id = ?", networkID)
		if err != nil {
			return err
		}
	}

	// Sync denied commands: upsert current config, delete orphaned ones
	if len(network.DenyCommands) > 0 {
		// Upsert denied commands from config using INSERT ... ON CONFLICT to preserve IDs
		for _, cmd := range network.DenyCommands {
			_, err = tx.Exec(`
				INSERT INTO denied_commands (network_id, channel_id, command)
				VALUES (?, NULL, ?)
				ON CONFLICT(network_id, channel_id, command) DO NOTHING
			`, networkID, cmd)
			if err != nil {
				return err
			}
		}

		// Delete denied commands not in current config
		placeholders := make([]string, len(network.DenyCommands))
		args := make([]interface{}, len(network.DenyCommands)+1)
		args[0] = networkID
		for i, cmd := range network.DenyCommands {
			placeholders[i] = "?"
			args[i+1] = cmd
		}

		deleteQuery := fmt.Sprintf(`
			DELETE FROM denied_commands 
			WHERE network_id = ? AND command NOT IN (%s)
		`, strings.Join(placeholders, ","))

		_, err = tx.Exec(deleteQuery, args...)
		if err != nil {
			return err
		}
	} else {
		// No denied commands in config, delete all for this network
		_, err = tx.Exec("DELETE FROM denied_commands WHERE network_id = ?", networkID)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func LoadNetwork(networkName string) (*NetworkData, error) {
	return Data.LoadNetwork(networkName)
}

func (s *SQLiteDB) LoadNetwork(networkName string) (*NetworkData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	network := &NetworkData{}

	// Load network data
	var networkID int64
	err := s.db.QueryRow(`
		SELECT id, enabled, nick, user_name, real_name, preserve_modes,
			ping_delay, version, throttle, burst, action_trigger, modes_at_once,
			nickserv_pass, server_pass
		FROM networks WHERE network_name = ?
	`, networkName).Scan(&networkID, &network.Enabled, &network.Nick, &network.User, &network.Name,
		&network.PreserveModes, &network.PingDelay, &network.Version, &network.Throttle,
		&network.Burst, &network.ActionTrigger, &network.ModesAtOnce, &network.NickServPass, &network.Pass)

	if err != nil {
		return nil, err
	}

	// Load servers
	rows, err := s.db.Query(`
		SELECT host, port, ssl, skip_ssl_verify
		FROM servers WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var server ServerData
		err = rows.Scan(&server.Host, &server.Port, &server.SSL, &server.SkipSSLVerify)
		if err != nil {
			return nil, err
		}
		network.Servers = append(network.Servers, server)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Load admin hosts
	rows, err = s.db.Query(`
		SELECT host, ident, is_owner
		FROM admin_hosts WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var admin AdminHost
		err = rows.Scan(&admin.Host, &admin.Ident, &admin.Owner)
		if err != nil {
			return nil, err
		}
		network.AdminHosts = append(network.AdminHosts, admin)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Load ignored nicks
	rows, err = s.db.Query(`
		SELECT nickname
		FROM ignored_nicks WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var nick string
		err = rows.Scan(&nick)
		if err != nil {
			return nil, err
		}
		network.IgnoredNicks = append(network.IgnoredNicks, nick)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	// Load denied commands
	rows, err = s.db.Query(`
		SELECT command
		FROM denied_commands WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var cmd string
		err = rows.Scan(&cmd)
		if err != nil {
			return nil, err
		}
		network.DenyCommands = append(network.DenyCommands, cmd)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return network, nil
}

// LoadNetworkChannels loads just the channel data for a network (for sync analysis)
func LoadNetworkChannels(networkName string) ([]ChannelData, error) {
	return Data.LoadNetworkChannels(networkName)
}

func (s *SQLiteDB) LoadNetworkChannels(networkName string) ([]ChannelData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get network ID
	var networkID int64
	err := s.db.QueryRow("SELECT id FROM networks WHERE network_name = ?", networkName).Scan(&networkID)
	if err != nil {
		return nil, err
	}

	// Load channels
	rows, err := s.db.Query(`
		SELECT name, preserve_modes, ai_enabled, sd_enabled, image_describe, 
		       sound_enabled, video_enabled, action_trigger, trim_output
		FROM channels WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []ChannelData
	for rows.Next() {
		var ch ChannelData
		err = rows.Scan(&ch.Name, &ch.PreserveModes, &ch.Ai, &ch.Sd, &ch.ImageDescribe,
			&ch.Sound, &ch.Video, &ch.ActionTrigger, &ch.TrimOutput)
		if err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return channels, nil
}

// DeleteChannel removes a channel and all its related data (safe due to cascading foreign keys)
func DeleteChannel(networkName, channelName string) error {
	return Data.DeleteChannel(networkName, channelName)
}

func (s *SQLiteDB) DeleteChannel(networkName, channelName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		DELETE FROM channels 
		WHERE id IN (
			SELECT c.id FROM channels c
			JOIN networks n ON c.network_id = n.id
			WHERE n.network_name = ? AND c.name = ?
		)
	`, networkName, channelName)

	if err == nil {
		rowsAffected, _ := result.RowsAffected()
		logger.Debug("Deleted channel", "network", networkName, "channel", channelName, "rows_affected", rowsAffected)
	}

	return err
}

// DeleteNetwork removes a network and all its related data (safe due to cascading foreign keys)
func DeleteNetwork(networkName string) error {
	return Data.DeleteNetwork(networkName)
}

func (s *SQLiteDB) DeleteNetwork(networkName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`DELETE FROM networks WHERE network_name = ?`, networkName)

	if err == nil {
		rowsAffected, _ := result.RowsAffected()
		logger.Debug("Deleted network", "network", networkName, "rows_affected", rowsAffected)
	}

	return err
}

// GetAllNetworkNames returns all network names from database (for cleanup detection)
func GetAllNetworkNames() ([]string, error) {
	return Data.GetAllNetworkNames()
}

func (s *SQLiteDB) GetAllNetworkNames() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT network_name FROM networks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var networks []string
	for rows.Next() {
		var networkName string
		err = rows.Scan(&networkName)
		if err != nil {
			return nil, err
		}
		networks = append(networks, networkName)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return networks, nil
}

// User Methods
func SaveNetworkUsers(networkName string, users []UserData) error {
	return Data.SaveNetworkUsers(networkName, users)
}

// SaveNetworkUsersWithChannels saves users (channels are handled by SaveNetwork)
func SaveNetworkUsersWithChannels(networkName string, users []UserData, channels []ChannelData) error {
	return Data.SaveNetworkUsersWithChannels(networkName, users, channels)
}

func (s *SQLiteDB) SaveNetworkUsersWithChannels(networkName string, users []UserData, channels []ChannelData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get or create network ID
	var networkID int64
	err = tx.QueryRow("SELECT id FROM networks WHERE network_name = ?", networkName).Scan(&networkID)
	if err == sql.ErrNoRows {
		// Create minimal network entry for user association
		err = tx.QueryRow(`
			INSERT INTO networks (network_name, enabled, nick) 
			VALUES (?, 1, 'placeholder') 
			RETURNING id
		`, networkName).Scan(&networkID)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// Channels are now managed by SaveNetwork - no need to create them here

	// Save users to database
	return s.saveUsersToDatabase(tx, networkID, users)
}

func (s *SQLiteDB) SaveNetworkUsers(networkName string, users []UserData) error {
	// Legacy method - call the new method with empty channels
	return s.SaveNetworkUsersWithChannels(networkName, users, []ChannelData{})
}

// SaveSingleUser saves just one user to the database (optimized for individual user updates)
func SaveSingleUser(networkName, ident, host string, user *UserData) error {
	return Data.SaveSingleUser(networkName, ident, host, user)
}

func (s *SQLiteDB) SaveSingleUser(networkName, ident, host string, user *UserData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get network ID
	var networkID int64
	err = tx.QueryRow("SELECT id FROM networks WHERE network_name = ?", networkName).Scan(&networkID)
	if err != nil {
		return fmt.Errorf("network not found: %w", err)
	}

	// Upsert the single user
	var userID int64
	err = tx.QueryRow(`
		INSERT INTO irc_users (network_id, nickname, ident, host, first_seen, latest_activity,
			latest_chat, is_admin, is_owner, ignored, access_level, ai_service, ai_model,
			ai_base_prompt, ai_personality)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(network_id, ident, host) DO UPDATE SET
			nickname = excluded.nickname,
			latest_activity = excluded.latest_activity,
			latest_chat = excluded.latest_chat,
			is_admin = excluded.is_admin,
			is_owner = excluded.is_owner,
			ignored = excluded.ignored,
			access_level = excluded.access_level,
			ai_service = excluded.ai_service,
			ai_model = excluded.ai_model,
			ai_base_prompt = excluded.ai_base_prompt,
			ai_personality = excluded.ai_personality,
			updated_at = datetime('now')
		RETURNING id
	`, networkID, user.NickName, user.Ident, user.Host, user.FirstSeen, user.LatestActivity,
		user.LatestChat, user.IsAdmin, user.IsOwner, user.Ignored, user.AccessLevel,
		user.AiService, user.AiModel, user.AiBasePrompt, user.AiPersonality).Scan(&userID)

	if err != nil {
		return err
	}

	// Save user modes if any
	for _, modeData := range user.PreservedModes {
		modesJSON, jsonErr := json.Marshal(modeData.Modes)
		if jsonErr != nil {
			return jsonErr
		}

		// Get channel ID
		var channelID int64
		err = tx.QueryRow("SELECT id FROM channels WHERE network_id = ? AND name = ?", networkID, modeData.Channel).Scan(&channelID)
		if err != nil {
			continue // Skip if channel doesn't exist
		}

		_, err = tx.Exec(`
			INSERT INTO user_modes (user_id, channel_id, mode_type, modes)
			VALUES (?, ?, 'preserved', ?)
			ON CONFLICT(user_id, channel_id, mode_type) DO UPDATE SET
				modes = excluded.modes,
				updated_at = datetime('now')
		`, userID, channelID, modesJSON)
		if err != nil {
			return err
		}
	}

	for _, modeData := range user.CurrentModes {
		modesJSON, jsonErr := json.Marshal(modeData.Modes)
		if jsonErr != nil {
			return jsonErr
		}

		// Get channel ID
		var channelID int64
		err = tx.QueryRow("SELECT id FROM channels WHERE network_id = ? AND name = ?", networkID, modeData.Channel).Scan(&channelID)
		if err != nil {
			continue // Skip if channel doesn't exist
		}

		_, err = tx.Exec(`
			INSERT INTO user_modes (user_id, channel_id, mode_type, modes)
			VALUES (?, ?, 'current', ?)
			ON CONFLICT(user_id, channel_id, mode_type) DO UPDATE SET
				modes = excluded.modes,
				updated_at = datetime('now')
		`, userID, channelID, modesJSON)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteDB) saveUsersToDatabase(tx *sql.Tx, networkID int64, users []UserData) error {
	logger.Debug("saveUsersToDatabase called", "network_id", networkID, "users_count", len(users))

	// Deduplicate users by ident@host (matching database UNIQUE constraint)
	// When users change nicks, they keep the same ident@host but get new nickname
	// We keep the user entry with the latest activity (most recent nick)
	userMap := make(map[string]*UserData)
	for i, user := range users {
		key := fmt.Sprintf("%s@%s", user.Ident, user.Host)
		if existing, exists := userMap[key]; exists {
			logger.Warn("Duplicate user detected (same ident@host), keeping latest activity", "network_id", networkID, "index", i, "existing_nick", existing.NickName, "new_nick", user.NickName, "ident", user.Ident, "host", user.Host)
			// Keep the user with latest activity (most recent nick change)
			if user.LatestActivity > existing.LatestActivity {
				logger.Debug("Keeping newer nick", "old_nick", existing.NickName, "new_nick", user.NickName, "ident", user.Ident, "host", user.Host)
				userMap[key] = &users[i]
			} else {
				logger.Debug("Keeping older nick", "kept_nick", existing.NickName, "discarded_nick", user.NickName, "ident", user.Ident, "host", user.Host)
			}
		} else {
			userMap[key] = &users[i]
		}
	}

	// Convert back to slice
	deduplicatedUsers := make([]UserData, 0, len(userMap))
	for _, user := range userMap {
		deduplicatedUsers = append(deduplicatedUsers, *user)
	}

	logger.Debug("Deduplicated users", "network_id", networkID, "original_count", len(users), "deduplicated_count", len(deduplicatedUsers))

	// Upsert users (update existing based on network_id+ident+host, insert new)
	// This preserves foreign key relationships and prevents orphaned data
	for i, user := range deduplicatedUsers {
		logger.Debug("Upserting user", "network_id", networkID, "index", i, "nickname", user.NickName, "ident", user.Ident, "host", user.Host)

		// Use INSERT OR REPLACE based on unique constraint (network_id, ident, host)
		var userID int64
		err := tx.QueryRow(`
			INSERT INTO irc_users (network_id, nickname, ident, host, first_seen, latest_activity,
				latest_chat, is_admin, is_owner, ignored, access_level, ai_service, ai_model,
				ai_base_prompt, ai_personality)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(network_id, ident, host) DO UPDATE SET
				nickname = excluded.nickname,
				latest_activity = excluded.latest_activity,
				latest_chat = excluded.latest_chat,
				is_admin = excluded.is_admin,
				is_owner = excluded.is_owner,
				ignored = excluded.ignored,
				access_level = excluded.access_level,
				ai_service = excluded.ai_service,
				ai_model = excluded.ai_model,
				ai_base_prompt = excluded.ai_base_prompt,
				ai_personality = excluded.ai_personality,
				updated_at = datetime('now')
			RETURNING id
		`, networkID, user.NickName, user.Ident, user.Host, user.FirstSeen, user.LatestActivity,
			user.LatestChat, user.IsAdmin, user.IsOwner, user.Ignored, user.AccessLevel,
			user.AiService, user.AiModel, user.AiBasePrompt, user.AiPersonality).Scan(&userID)

		if err != nil {
			return err
		}

		// Insert user modes - ensure channel exists first
		for _, modeData := range user.PreservedModes {
			modesJSON, jsonErr := json.Marshal(modeData.Modes)
			if jsonErr != nil {
				return jsonErr
			}

			// Ensure channel exists (create minimal entry if needed)
			_, err = tx.Exec(`
				INSERT OR IGNORE INTO channels (network_id, name) VALUES (?, ?)
			`, networkID, modeData.Channel)
			if err != nil {
				return err
			}

			var channelID int64
			err = tx.QueryRow(`
				SELECT id FROM channels WHERE network_id = ? AND name = ?
			`, networkID, modeData.Channel).Scan(&channelID)
			if err != nil {
				return err
			}

			_, err = tx.Exec(`
				INSERT INTO user_modes (user_id, channel_id, mode_type, modes)
				VALUES (?, ?, 'preserved', ?)
				ON CONFLICT(user_id, channel_id, mode_type) DO UPDATE SET
					modes = excluded.modes,
					updated_at = datetime('now')
			`, userID, channelID, modesJSON)
			if err != nil {
				return err
			}
		}

		for _, modeData := range user.CurrentModes {
			modesJSON, jsonErr := json.Marshal(modeData.Modes)
			if jsonErr != nil {
				return jsonErr
			}

			var channelID int64
			err = tx.QueryRow(`
				SELECT id FROM channels WHERE network_id = ? AND name = ?
			`, networkID, modeData.Channel).Scan(&channelID)
			if err != nil {
				return err
			}

			_, err = tx.Exec(`
				INSERT INTO user_modes (user_id, channel_id, mode_type, modes)
				VALUES (?, ?, 'current', ?)
				ON CONFLICT(user_id, channel_id, mode_type) DO UPDATE SET
					modes = excluded.modes,
					updated_at = datetime('now')
			`, userID, channelID, modesJSON)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func LoadNetworkUsers(networkName string) ([]UserData, error) {
	return Data.LoadNetworkUsers(networkName)
}

func (s *SQLiteDB) LoadNetworkUsers(networkName string) ([]UserData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get network ID
	var networkID int64
	err := s.db.QueryRow("SELECT id FROM networks WHERE network_name = ?", networkName).Scan(&networkID)
	if err != nil {
		return nil, err
	}

	// Load users
	rows, err := s.db.Query(`
		SELECT id, nickname, ident, host, first_seen, latest_activity, latest_chat,
			is_admin, is_owner, ignored, access_level, ai_service, ai_model,
			ai_base_prompt, ai_personality
		FROM irc_users WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserData
	for rows.Next() {
		var user UserData
		err = rows.Scan(&user.ID, &user.NickName, &user.Ident, &user.Host, &user.FirstSeen,
			&user.LatestActivity, &user.LatestChat, &user.IsAdmin, &user.IsOwner, &user.Ignored,
			&user.AccessLevel, &user.AiService, &user.AiModel, &user.AiBasePrompt, &user.AiPersonality)
		if err != nil {
			return nil, err
		}
		user.NetworkID = int(networkID)

		// Load user modes with proper channel names
		modeRows, modeErr := s.db.Query(`
			SELECT um.mode_type, um.modes, c.name as channel_name
			FROM user_modes um
			JOIN channels c ON um.channel_id = c.id
			WHERE um.user_id = ?
		`, user.ID)
		if modeErr != nil {
			return nil, modeErr
		}

		for modeRows.Next() {
			var modeType string
			var modesJSON []byte
			var channelName string
			err = modeRows.Scan(&modeType, &modesJSON, &channelName)
			if err != nil {
				modeRows.Close()
				return nil, err
			}

			var modes []string
			if jsonErr := json.Unmarshal(modesJSON, &modes); jsonErr != nil {
				modeRows.Close()
				return nil, jsonErr
			}

			modeData := UserModeData{
				Channel: channelName,
				Modes:   modes,
			}

			if modeType == "preserved" {
				user.PreservedModes = append(user.PreservedModes, modeData)
			} else {
				user.CurrentModes = append(user.CurrentModes, modeData)
			}
		}
		modeRows.Close()

		users = append(users, user)
	}

	return users, nil
}

// Get user by ident and host (most common lookup)
func GetUserByIdentHost(networkName, ident, host string) (*UserData, error) {
	return Data.GetUserByIdentHost(networkName, ident, host)
}

func (s *SQLiteDB) GetUserByIdentHost(networkName, ident, host string) (*UserData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var user UserData
	err := s.db.QueryRow(`
		SELECT u.id, u.nickname, u.ident, u.host, u.first_seen, u.latest_activity, u.latest_chat,
			u.is_admin, u.is_owner, u.ignored, u.access_level, u.ai_service, u.ai_model,
			u.ai_base_prompt, u.ai_personality, n.id as network_id
		FROM irc_users u
		JOIN networks n ON u.network_id = n.id
		WHERE n.network_name = ? AND u.ident = ? AND u.host = ?
	`, networkName, ident, host).Scan(&user.ID, &user.NickName, &user.Ident, &user.Host,
		&user.FirstSeen, &user.LatestActivity, &user.LatestChat, &user.IsAdmin, &user.IsOwner,
		&user.Ignored, &user.AccessLevel, &user.AiService, &user.AiModel, &user.AiBasePrompt,
		&user.AiPersonality, &user.NetworkID)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

// User-Channel relationship management
func AddUserToChannel(networkName, ident, host, channelName string) error {
	return Data.AddUserToChannel(networkName, ident, host, channelName)
}

func (s *SQLiteDB) AddUserToChannel(networkName, ident, host, channelName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get user and channel IDs
	var userID, channelID int64

	err := s.db.QueryRow(`
		SELECT u.id, c.id 
		FROM irc_users u
		JOIN networks n ON u.network_id = n.id
		JOIN channels c ON c.network_id = n.id
		WHERE n.network_name = ? AND u.ident = ? AND u.host = ? AND c.name = ?
	`, networkName, ident, host, channelName).Scan(&userID, &channelID)

	if err != nil {
		// User or channel doesn't exist yet - this is normal for new users
		logger.Debug("Cannot add user to channel - user or channel not found", "network", networkName, "ident", ident, "host", host, "channel", channelName, "error", err)
		return nil // Don't treat this as an error
	}

	// Add user to channel (ignore if already exists)
	_, err = s.db.Exec(`
		INSERT OR IGNORE INTO user_channels (user_id, channel_id, joined_at)
		VALUES (?, ?, datetime('now'))
	`, userID, channelID)

	if err == nil {
		logger.Debug("Added user to channel", "network", networkName, "ident", ident, "host", host, "channel", channelName)
	}

	return err
}

func RemoveUserFromChannel(networkName, ident, host, channelName string) error {
	return Data.RemoveUserFromChannel(networkName, ident, host, channelName)
}

func (s *SQLiteDB) RemoveUserFromChannel(networkName, ident, host, channelName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		DELETE FROM user_channels 
		WHERE user_id IN (
			SELECT u.id FROM irc_users u
			JOIN networks n ON u.network_id = n.id
			WHERE n.network_name = ? AND u.ident = ? AND u.host = ?
		) AND channel_id IN (
			SELECT c.id FROM channels c
			JOIN networks n ON c.network_id = n.id
			WHERE n.network_name = ? AND c.name = ?
		)
	`, networkName, ident, host, networkName, channelName)

	if err == nil {
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			logger.Debug("Removed user from channel", "network", networkName, "ident", ident, "host", host, "channel", channelName)
		}
	}

	return err
}

func RemoveUserFromAllChannels(networkName, ident, host string) error {
	return Data.RemoveUserFromAllChannels(networkName, ident, host)
}

func (s *SQLiteDB) RemoveUserFromAllChannels(networkName, ident, host string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		DELETE FROM user_channels 
		WHERE user_id IN (
			SELECT u.id FROM irc_users u
			JOIN networks n ON u.network_id = n.id
			WHERE n.network_name = ? AND u.ident = ? AND u.host = ?
		)
	`, networkName, ident, host)

	if err == nil {
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			logger.Debug("Removed user from all channels", "network", networkName, "ident", ident, "host", host, "channels_left", rowsAffected)
		}
	}

	return err
}

// Update user activity (common operation)
func UpdateUserActivity(networkName, ident, host string, activity int64, latestChat string) error {
	return Data.UpdateUserActivity(networkName, ident, host, activity, latestChat)
}

func (s *SQLiteDB) UpdateUserActivity(networkName, ident, host string, activity int64, latestChat string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE irc_users SET 
			latest_activity = ?, 
			latest_chat = ?,
			updated_at = datetime('now')
		WHERE network_id = (SELECT id FROM networks WHERE network_name = ?)
			AND ident = ? AND host = ?
	`, activity, latestChat, networkName, ident, host)

	return err
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
