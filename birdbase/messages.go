package birdbase

import (
	"database/sql"
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

func initMessageWorker() {
	messageChan = make(chan storedMessage, messageBufferSize)
	go messageWriteWorker()
}

func messageWriteWorker() {
	batch := make([]storedMessage, 0, batchSize)
	timer := time.NewTimer(batchTimeout)
	defer timer.Stop()

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
		return cached.(int64)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var id int64
	err := s.db.QueryRow("SELECT id FROM networks WHERE network_name = ?", networkName).Scan(&id)
	if err != nil {
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
	default:
		// Buffer full, drop message rather than block
		logger.Warn("Message storage buffer full, dropping message", "channel", channelName, "nick", nickname)
	}
}

// GetChannelMessages retrieves messages for a channel within the given hours, capped at maxMessages.
// Returns messages in chronological order (oldest first).
func GetChannelMessages(networkID int64, channelName string, hours, maxMessages int) ([]ChannelMessage, error) {
	return Data.GetChannelMessages(networkID, channelName, hours, maxMessages)
}

func (s *SQLiteDB) GetChannelMessages(networkID int64, channelName string, hours, maxMessages int) ([]ChannelMessage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if maxMessages <= 0 {
		maxMessages = 200
	}

	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()

	// Get total count first to know if we need to subsample
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM channel_messages
		WHERE network_id = ? AND channel_name = ? AND timestamp >= ?
	`, networkID, strings.ToLower(channelName), cutoff).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to count channel messages: %w", err)
	}

	if count == 0 {
		return nil, nil
	}

	// If we have more than maxMessages, take the most recent ones
	query := `
		SELECT nickname, event_type, message, timestamp FROM channel_messages
		WHERE network_id = ? AND channel_name = ? AND timestamp >= ?
		ORDER BY timestamp ASC
	`

	var rows *sql.Rows
	if count > maxMessages {
		// Use a subquery to get only the most recent maxMessages
		query = `
			SELECT nickname, event_type, message, timestamp FROM (
				SELECT nickname, event_type, message, timestamp FROM channel_messages
				WHERE network_id = ? AND channel_name = ? AND timestamp >= ?
				ORDER BY timestamp DESC
				LIMIT ?
			) ORDER BY timestamp ASC
		`
		rows, err = s.db.Query(query, networkID, strings.ToLower(channelName), cutoff, maxMessages)
	} else {
		rows, err = s.db.Query(query, networkID, strings.ToLower(channelName), cutoff)
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

	rows, err := s.db.Query(`
		SELECT nickname, event_type, message, timestamp FROM channel_messages
		WHERE network_id = ? AND channel_name = ? AND message LIKE ? AND event_type IN ('privmsg', 'action')
		ORDER BY timestamp DESC
		LIMIT ?
	`, networkID, strings.ToLower(channelName), "%"+keyword+"%", maxResults)
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
		if err := chatterRows.Scan(&entry.Nickname, &entry.Count); err != nil {
			continue
		}
		stats.TopChatters = append(stats.TopChatters, entry)
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
		if err := eventRows.Scan(&eventType, &count); err != nil {
			continue
		}
		stats.EventCounts[eventType] = count
	}

	return stats, nil
}
