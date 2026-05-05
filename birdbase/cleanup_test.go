package birdbase

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanup_RemovesExpiredRows(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	// Insert an expired chat_history row with backdated expires_at
	_, err = db.db.Exec(`
		INSERT INTO chat_history (cache_key, messages, expires_at, updated_at)
		VALUES ('expired-chat', '[{"role":"user","content":"hi"}]', datetime('now', '-1 second'), datetime('now'))
	`)
	require.NoError(t, err)

	// Insert an expired key_value_store row
	_, err = db.db.Exec(`
		INSERT INTO key_value_store (key_name, value_data, expires_at, updated_at)
		VALUES ('expired-key', 'old-value', datetime('now', '-1 second'), datetime('now'))
	`)
	require.NoError(t, err)

	// Insert a non-expired key_value_store row (null expires_at = permanent)
	_, err = db.db.Exec(`
		INSERT INTO key_value_store (key_name, value_data, expires_at, updated_at)
		VALUES ('permanent-key', 'permanent-value', NULL, datetime('now'))
	`)
	require.NoError(t, err)

	// Verify rows exist before cleanup
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM chat_history").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	err = db.db.QueryRow("SELECT COUNT(*) FROM key_value_store").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	// Run cleanup
	err = db.Cleanup()
	require.NoError(t, err)

	// Verify expired rows are deleted
	err = db.db.QueryRow("SELECT COUNT(*) FROM chat_history").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Only the permanent key should remain
	err = db.db.QueryRow("SELECT COUNT(*) FROM key_value_store").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify permanent key is still accessible
	var value string
	err = db.db.QueryRow("SELECT value_data FROM key_value_store WHERE key_name = 'permanent-key'").Scan(&value)
	require.NoError(t, err)
	assert.Equal(t, "permanent-value", value)
}

func TestCleanup_NoExpiredRows(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	// Insert only permanent rows
	_, err = db.db.Exec(`
		INSERT INTO key_value_store (key_name, value_data, expires_at, updated_at)
		VALUES ('key1', 'val1', NULL, datetime('now'))
	`)
	require.NoError(t, err)

	err = db.Cleanup()
	require.NoError(t, err)

	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM key_value_store").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestVacuum(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	// Vacuum should succeed even on empty DB
	err = db.vacuum()
	require.NoError(t, err)
}

func TestCleanup_OldChannelMessages(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	// Insert a network row to satisfy the foreign key constraint
	_, err = db.db.Exec(`
		INSERT INTO networks (network_name, enabled, nick)
		VALUES ('testnet', 1, 'testnick')
	`)
	require.NoError(t, err)

	// Insert an old channel message (8 days ago)
	oldTimestamp := time.Now().AddDate(0, 0, -8).Unix()
	_, err = db.db.Exec(`
		INSERT INTO channel_messages (network_id, channel_name, nickname, event_type, message, timestamp)
		VALUES (1, '#test', 'user1', 'privmsg', 'old message', ?)
	`, oldTimestamp)
	require.NoError(t, err)

	// Insert a recent channel message (1 day ago)
	recentTimestamp := time.Now().AddDate(0, 0, -1).Unix()
	_, err = db.db.Exec(`
		INSERT INTO channel_messages (network_id, channel_name, nickname, event_type, message, timestamp)
		VALUES (1, '#test', 'user2', 'privmsg', 'recent message', ?)
	`, recentTimestamp)
	require.NoError(t, err)

	// Run cleanup with 7-day retention
	err = db.Cleanup()
	require.NoError(t, err)

	// Only the recent message should remain
	var count int
	err = db.db.QueryRow("SELECT COUNT(*) FROM channel_messages").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	var msg string
	err = db.db.QueryRow("SELECT message FROM channel_messages").Scan(&msg)
	require.NoError(t, err)
	assert.Equal(t, "recent message", msg)
}
