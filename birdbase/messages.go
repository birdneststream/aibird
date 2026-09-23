package birdbase

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"aibird/logger"
)

// networkIDCache caches network name → database ID to avoid per-message DB lookups.
var networkIDCache sync.Map

const (
	messageBufferSize = 500
	batchSize         = 25
	batchTimeout      = 2 * time.Second
)

// ChannelMessage represents a single stored IRC event for the summary feature.
type ChannelMessage struct {
	Nickname  string
	EventType string
	Message   string
	Timestamp int64
}

// storedMessage is the internal struct sent through the write channel.
type storedMessage struct {
	NetworkID   int64
	ChannelName string
	Nickname    string
	EventType   string
	Message     string
	Timestamp   int64
}

var messageChan chan storedMessage

// messageWorkerDone is closed by messageWriteWorker when it exits,
// allowing tests to wait for clean shutdown.
var messageWorkerDone chan struct{}

// pendingSeen records the newest event seen per (network, channel, nick) as
// soon as StoreChannelMessage accepts it, before the batched write worker has
// flushed the row to disk. Reads consult it so a message sent moments ago is
// never reported as "not seen" or attributed an older timestamp.
//
// Values are *pendingSeenEntry. It is a best-effort cache: entries are lost on
// restart, at which point the database becomes the sole source of truth.
var pendingSeen sync.Map

// pendingSeenEntry holds the newest activity known for a nick. It is mutated
// in place under mu so repeated lookups do not reallocate map entries.
type pendingSeenEntry struct {
	mu        sync.Mutex
	timestamp int64
	eventType string
}

// pendingSeenKey builds the map key. The channel and nick are lowercased
// because IRC channels and nicks are case-insensitive.
func pendingSeenKey(networkID int64, channelName, nickname string) string {
	return fmt.Sprintf("%d\x1f%s\x1f%s", networkID, strings.ToLower(channelName), strings.ToLower(nickname))
}

func initMessageWorker() {
	messageChan = make(chan storedMessage, messageBufferSize)
	messageWorkerDone = make(chan struct{})
	go messageWriteWorker()
}

func messageWriteWorker() {
	batch := make([]storedMessage, 0, batchSize)
	timer := time.NewTimer(batchTimeout)
	defer timer.Stop()
	defer close(messageWorkerDone)

	for {
		select {
		case msg, ok := <-messageChan:
			if !ok {
				// Channel closed, flush remaining
				if len(batch) > 0 {
					flushMessageBatch(batch)
				}
				return
			}
			batch = append(batch, msg)
			if len(batch) >= batchSize {
				flushMessageBatch(batch)
				batch = batch[:0]
				timer.Reset(batchTimeout)
			}

		case <-timer.C:
			if len(batch) > 0 {
				flushMessageBatch(batch)
				batch = batch[:0]
			}
			timer.Reset(batchTimeout)
		}
	}
}

func flushMessageBatch(batch []storedMessage) {
	if Data == nil || len(batch) == 0 {
		return
	}

	Data.mu.Lock()
	defer Data.mu.Unlock()

	tx, err := Data.db.Begin()
	if err != nil {
		logger.Error("Failed to begin message batch transaction", "error", err)
		return
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO channel_messages (network_id, channel_name, nickname, event_type, message, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		logger.Error("Failed to prepare message insert", "error", err)
		return
	}
	defer stmt.Close()

	for _, msg := range batch {
		_, err := stmt.Exec(msg.NetworkID, msg.ChannelName, msg.Nickname, msg.EventType, msg.Message, msg.Timestamp)
		if err != nil {
			logger.Error("Failed to insert channel message", "error", err, "channel", msg.ChannelName, "nick", msg.Nickname)
		}
	}

	if err := tx.Commit(); err != nil {
		logger.Error("Failed to commit message batch", "error", err)
	}
}

// CleanupOldMessages removes messages older than retentionDays. Called by the maintenance loop.
func CleanupOldMessages(retentionDays int) (int64, error) {
	return Data.CleanupOldMessages(retentionDays)
}

func (s *SQLiteDB) CleanupOldMessages(retentionDays int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cleanupOldMessagesLocked(retentionDays)
}

// cleanupOldMessagesLocked does the actual cleanup. Caller must hold s.mu.
func (s *SQLiteDB) cleanupOldMessagesLocked(retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = 7
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()

	result, err := s.db.Exec(`DELETE FROM channel_messages WHERE timestamp < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old channel messages: %w", err)
	}

	deleted, _ := result.RowsAffected()
	if deleted > 0 {
		logger.Info("Cleaned up old channel messages", "deleted", deleted, "retention_days", retentionDays)
	}
	return deleted, nil
}

// ResolveNetworkID looks up the database ID for a network name. Uses a sync.Map
// cache to avoid repeated DB lookups on every message.
func ResolveNetworkID(networkName string) int64 {
	return Data.ResolveNetworkID(networkName)
}

func (s *SQLiteDB) ResolveNetworkID(networkName string) int64 {
	// Check cache first
	if cached, ok := networkIDCache.Load(networkName); ok {
		return cached.(int64) //nolint:errcheck // cache only stores int64 values
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var id int64
	err := s.db.QueryRow("SELECT id FROM networks WHERE network_name = ?", networkName).Scan(&id)
	if err != nil {
		logger.Warn("Failed to resolve network ID", "network", networkName, "error", err)
		return 0
	}

	// Cache for future lookups
	networkIDCache.Store(networkName, id)
	return id
}

// StoreChannelMessage queues a message for asynchronous batched storage.
// networkID should be pre-resolved from in-memory state for performance.
// This function never blocks — if the buffer is full, the message is dropped.
func StoreChannelMessage(networkID int64, channelName, nickname, eventType, message string) {
	if messageChan == nil {
		return
	}

	msg := storedMessage{
		NetworkID:   networkID,
		ChannelName: strings.ToLower(channelName),
		Nickname:    nickname,
		EventType:   eventType,
		Message:     message,
		Timestamp:   time.Now().Unix(),
	}

	select {
	case messageChan <- msg:
		recordPendingSeen(&msg)
	default:
		// Buffer full, drop message rather than block
		logger.Warn("Message storage buffer full, dropping message", "channel", channelName, "nick", nickname)
	}
}

// pendingEntryFor returns the cache entry for the given key, creating it when
// absent. The second result is false only if the map held an unexpected type,
// which cannot happen as long as every writer stores *pendingSeenEntry.
func pendingEntryFor(key string, create bool) (*pendingSeenEntry, bool) {
	if !create {
		value, ok := pendingSeen.Load(key)
		if !ok {
			return nil, false
		}
		entry, ok := value.(*pendingSeenEntry)
		return entry, ok
	}

	value, _ := pendingSeen.LoadOrStore(key, &pendingSeenEntry{})
	entry, ok := value.(*pendingSeenEntry)
	return entry, ok
}

// recordPendingSeen updates the in-memory newest-activity cache for a message
// that was accepted into the write buffer. Older events arriving out of order
// (possible when the worker replays a batch) never move a timestamp backwards.
func recordPendingSeen(msg *storedMessage) {
	if msg == nil || msg.Nickname == "" {
		return
	}

	entry, ok := pendingEntryFor(pendingSeenKey(msg.NetworkID, msg.ChannelName, msg.Nickname), true)
	if !ok {
		return
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()
	if msg.Timestamp >= entry.timestamp {
		entry.timestamp = msg.Timestamp
		entry.eventType = msg.EventType
	}
}

// TouchUserActivity advances irc_users.latest_activity for a single user.
//
// It updates only that column rather than re-writing the whole row: the chat
// path fires on every line, and a full upsert would also carry mode data that
// other goroutines may be mutating concurrently. Timestamps only move forward,
// so an out-of-order call can never regress the stored value.
func TouchUserActivity(networkName, ident, host string, timestamp int64) error {
	return Data.TouchUserActivity(networkName, ident, host, timestamp)
}

func (s *SQLiteDB) TouchUserActivity(networkName, ident, host string, timestamp int64) error {
	if timestamp <= 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec(`
		UPDATE irc_users
		SET latest_activity = ?, updated_at = datetime('now')
		WHERE network_id = (SELECT id FROM networks WHERE network_name = ?)
		  AND ident = ? AND host = ?
		  AND latest_activity < ?
	`, timestamp, networkName, ident, host, timestamp)
	if err != nil {
		return fmt.Errorf("failed to update latest_activity for %s@%s: %w", ident, host, err)
	}

	// A missing row means we never saw this ident/host before, which the caller
	// treats as harmless — the join/WHO path will insert the user properly.
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		logger.Debug("No irc_users row to touch", "network", networkName, "ident", ident, "host", host)
	}

	return nil
}

// LastSeenResult reports when the bot last recorded activity for a nick.
type LastSeenResult struct {
	Timestamp int64  // Unix seconds; 0 when the nick has no recorded activity
	EventType string // privmsg, action, join, part, quit, kick
	Found     bool
}

// LastSeen returns the newest recorded activity for a nick on a channel.
//
// It reads the channel_messages table, which is written for every non-command
// chat message as well as join/part/quit/kick events, and then layers the
// in-memory pending cache on top so buffered-but-unflushed events still count.
// Matching on the nickname is case-insensitive, as IRC nicks are.
func LastSeen(networkID int64, channelName, nickname string) (LastSeenResult, error) {
	return Data.LastSeen(networkID, channelName, nickname)
}

func (s *SQLiteDB) LastSeen(networkID int64, channelName, nickname string) (LastSeenResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := LastSeenResult{}

	var dbTimestamp int64
	var dbEventType string
	err := s.db.QueryRow(`
		SELECT timestamp, event_type FROM channel_messages
		WHERE network_id = ? AND channel_name = ? AND lower(nickname) = lower(?)
		ORDER BY timestamp DESC
		LIMIT 1
	`, networkID, strings.ToLower(channelName), nickname).Scan(&dbTimestamp, &dbEventType)
	switch {
	case err == nil:
		result = LastSeenResult{Timestamp: dbTimestamp, EventType: dbEventType, Found: true}
	case !errors.Is(err, sql.ErrNoRows):
		return result, fmt.Errorf("failed to look up last seen for %q: %w", nickname, err)
	}

	// The pending cache can be newer than the table, never older in practice,
	// but compare explicitly so a partial flush cannot go backwards.
	if entry, ok := pendingEntryFor(pendingSeenKey(networkID, channelName, nickname), false); ok {
		entry.mu.Lock()
		pending := LastSeenResult{Timestamp: entry.timestamp, EventType: entry.eventType, Found: true}
		entry.mu.Unlock()

		if pending.Timestamp > result.Timestamp {
			return pending, nil
		}
	}

	return result, nil
}

// GetChannelMessages retrieves messages for a channel within the given hours, capped at maxMessages.
// Returns messages in chronological order (oldest first).
//
// When eventTypes is non-empty, only rows whose event_type matches one of the given
// values are returned (e.g. "privmsg", "action"). When empty, all event types are
// returned. The filter is applied to both the count and the select, so maxMessages
// caps the filtered result set rather than the raw set.
func GetChannelMessages(networkID int64, channelName string, hours, maxMessages int, eventTypes ...string) ([]ChannelMessage, error) {
	return Data.GetChannelMessages(networkID, channelName, hours, maxMessages, eventTypes...)
}

func (s *SQLiteDB) GetChannelMessages(networkID int64, channelName string, hours, maxMessages int, eventTypes ...string) ([]ChannelMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if maxMessages <= 0 {
		maxMessages = 200
	}

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()

	// Build the optional event-type filter once; it is reused by the count and
	// select queries so maxMessages caps the filtered result set consistently.
	// filterClause is a static, parameterized "AND event_type IN (...)" fragment:
	// only placeholders are appended and every value is bound as an arg below.
	filterClause, filterArgs := buildEventTypeFilter(eventTypes)
	whereArgs := append([]any{networkID, strings.ToLower(channelName), cutoff}, filterArgs...)

	// Get total count first to know if we need to subsample
	var count int
	err := s.db.QueryRow(
		"SELECT COUNT(*) FROM channel_messages WHERE network_id = ? AND channel_name = ? AND timestamp >= ?"+filterClause,
		whereArgs...,
	).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to count channel messages: %w", err)
	}

	if count == 0 {
		return nil, nil
	}

	var rows *sql.Rows
	if count > maxMessages {
		// Use a subquery to get only the most recent maxMessages.
		// #nosec G202 - filterClause is a static, parameterized "AND event_type IN (...)" fragment from buildEventTypeFilter; only placeholders are appended and every value is bound as an arg.
		query := `
			SELECT nickname, event_type, message, timestamp FROM (
				SELECT nickname, event_type, message, timestamp FROM channel_messages
				WHERE network_id = ? AND channel_name = ? AND timestamp >= ?` + filterClause + `
				ORDER BY timestamp DESC
				LIMIT ?
			) ORDER BY timestamp ASC
		`
		rows, err = s.db.Query(query, append(whereArgs, maxMessages)...)
	} else {
		// #nosec G202 - filterClause is a static, parameterized "AND event_type IN (...)" fragment from buildEventTypeFilter; only placeholders are appended and every value is bound as an arg.
		query := `
			SELECT nickname, event_type, message, timestamp FROM channel_messages
			WHERE network_id = ? AND channel_name = ? AND timestamp >= ?` + filterClause + `
			ORDER BY timestamp ASC
		`
		rows, err = s.db.Query(query, whereArgs...)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query channel messages: %w", err)
	}
	defer rows.Close()

	var messages []ChannelMessage
	for rows.Next() {
		var msg ChannelMessage
		if err := rows.Scan(&msg.Nickname, &msg.EventType, &msg.Message, &msg.Timestamp); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

// buildEventTypeFilter returns an " AND event_type IN (?, ...)" clause for the
// given event types along with its bound arguments, or ("", nil) when no filter
// is requested. Only the static clause text is interpolated; the values are
// always passed as parameterized placeholders, so callers must not inject the
// event types into the SQL string directly.
func buildEventTypeFilter(eventTypes []string) (clause string, args []any) {
	if len(eventTypes) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(eventTypes))
	args = make([]any, len(eventTypes))
	for i, et := range eventTypes {
		placeholders[i] = "?"
		args[i] = et
	}
	return " AND event_type IN (" + strings.Join(placeholders, ", ") + ")", args
}

// SearchChannelMessages searches for messages containing a keyword in a channel.
// Returns up to maxResults messages in chronological order.
func SearchChannelMessages(networkID int64, channelName, keyword string, maxResults int) ([]ChannelMessage, error) {
	return Data.SearchChannelMessages(networkID, channelName, keyword, maxResults)
}

func (s *SQLiteDB) SearchChannelMessages(networkID int64, channelName, keyword string, maxResults int) ([]ChannelMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if maxResults <= 0 {
		maxResults = 10
	}

	// Escape special LIKE pattern characters to prevent injection
	escapedKeyword := escapeLike(keyword)
	pattern := "%" + escapedKeyword + "%"

	rows, err := s.db.Query(`
		SELECT nickname, event_type, message, timestamp FROM channel_messages
		WHERE network_id = ? AND channel_name = ? AND message LIKE ? ESCAPE '\' AND event_type IN ('privmsg', 'action')
		ORDER BY timestamp DESC
		LIMIT ?
	`, networkID, strings.ToLower(channelName), pattern, maxResults)
	if err != nil {
		return nil, fmt.Errorf("failed to search channel messages: %w", err)
	}
	defer rows.Close()

	var messages []ChannelMessage
	for rows.Next() {
		var msg ChannelMessage
		if err := rows.Scan(&msg.Nickname, &msg.EventType, &msg.Message, &msg.Timestamp); err != nil {
			continue
		}
		messages = append(messages, msg)
	}

	// Reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, rows.Err()
}

// ChannelStats holds aggregated statistics for a channel.
type ChannelStats struct {
	TotalMessages int
	TopChatters   []ChatterEntry
	EventCounts   map[string]int
}

// ChatterEntry represents a single user's activity count.
type ChatterEntry struct {
	Nickname string
	Count    int
}

// GetChannelStats returns activity statistics for a channel over the given hours.
func GetChannelStats(networkID int64, channelName string, hours int) (*ChannelStats, error) {
	return Data.GetChannelStats(networkID, channelName, hours)
}

func (s *SQLiteDB) GetChannelStats(networkID int64, channelName string, hours int) (*ChannelStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if hours <= 0 {
		hours = 24
	}

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
	chName := strings.ToLower(channelName)

	stats := &ChannelStats{
		EventCounts: make(map[string]int),
	}

	// Total message count
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM channel_messages
		WHERE network_id = ? AND channel_name = ? AND timestamp >= ?
	`, networkID, chName, cutoff).Scan(&stats.TotalMessages)
	if err != nil {
		return nil, fmt.Errorf("failed to get message count: %w", err)
	}

	if stats.TotalMessages == 0 {
		return stats, nil
	}

	// Top chatters (privmsg + action only)
	chatterRows, err := s.db.Query(`
		SELECT nickname, COUNT(*) as cnt FROM channel_messages
		WHERE network_id = ? AND channel_name = ? AND timestamp >= ? AND event_type IN ('privmsg', 'action')
		GROUP BY nickname
		ORDER BY cnt DESC
		LIMIT 10
	`, networkID, chName, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to get top chatters: %w", err)
	}
	defer chatterRows.Close()

	for chatterRows.Next() {
		var entry ChatterEntry
		if err = chatterRows.Scan(&entry.Nickname, &entry.Count); err != nil {
			continue
		}
		stats.TopChatters = append(stats.TopChatters, entry)
	}
	if err = chatterRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate top chatters: %w", err)
	}

	// Event type breakdown
	eventRows, err := s.db.Query(`
		SELECT event_type, COUNT(*) FROM channel_messages
		WHERE network_id = ? AND channel_name = ? AND timestamp >= ?
		GROUP BY event_type
	`, networkID, chName, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to get event counts: %w", err)
	}
	defer eventRows.Close()

	for eventRows.Next() {
		var eventType string
		var count int
		if scanErr := eventRows.Scan(&eventType, &count); scanErr != nil {
			continue
		}
		stats.EventCounts[eventType] = count
	}
	if err = eventRows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate event counts: %w", err)
	}

	return stats, nil
}

// escapeLike escapes special characters in a SQL LIKE pattern (%, _, \).
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
