package birdbase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSQLiteDB_CreatesSchema(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	// Verify core tables exist by querying them
	tables := []string{
		"user_usage",
		"command_leaderboard",
		"key_value_store",
		"networks",
		"channels",
		"irc_users",
		"user_modes",
		"user_channels",
		"servers",
		"admin_hosts",
		"ignored_nicks",
		"denied_commands",
		"channel_messages",
		"schema_version",
	}

	for _, table := range tables {
		var name string
		err := db.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		require.NoError(t, err, "table %s should exist", table)
		assert.Equal(t, table, name)
	}
}

func TestStats_EmptyDB(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	stats, err := db.Stats()
	require.NoError(t, err)

	keys, ok := stats["keys"].(int64)
	require.True(t, ok, "keys should be int64")
	assert.Equal(t, int64(0), keys)

	size, ok := stats["size"].(int64)
	require.True(t, ok, "size should be int64")
	assert.GreaterOrEqual(t, size, int64(0), "size should be non-negative")
}

func TestStats_WithKeys(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	// Insert some key-value pairs
	err = db.Put("key1", []byte("value1"))
	require.NoError(t, err)
	err = db.Put("key2", []byte("value2"))
	require.NoError(t, err)

	stats, err := db.Stats()
	require.NoError(t, err)

	keys, ok := stats["keys"].(int64)
	require.True(t, ok)
	assert.Equal(t, int64(2), keys)

	size, ok := stats["size"].(int64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, size, int64(0), "size should be non-negative")
}

func TestStats_ExcludesExpiredKeys(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	// Insert a permanent key and an expired key
	err = db.Put("permanent", []byte("value"))
	require.NoError(t, err)

	_, err = db.db.Exec(`
		INSERT INTO key_value_store (key_name, value_data, expires_at, updated_at)
		VALUES ('expired', 'old', datetime('now', '-1 second'), datetime('now'))
	`)
	require.NoError(t, err)

	stats, err := db.Stats()
	require.NoError(t, err)

	keys, ok := stats["keys"].(int64)
	require.True(t, ok)
	assert.Equal(t, int64(1), keys, "only permanent key should be counted")
}
