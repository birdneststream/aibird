package text

import (
	"fmt"
	"testing"

	"aibird/birdbase"
	"aibird/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logger.Init(logger.Config{Level: logger.LevelError, Format: "text"})
}

func setupTestDB(t *testing.T) {
	t.Helper()
	var err error
	birdbase.Data, err = birdbase.NewSQLiteDB(":memory:")
	require.NoError(t, err)
}

func TestAppendChatCache(t *testing.T) {
	setupTestDB(t)

	key := "test-append-cache"

	AppendChatCache(key, "user", "hello", 5)
	cache := GetChatCache(key)
	require.Len(t, cache, 1)
	assert.Equal(t, "user", cache[0].Role)
	assert.Equal(t, "hello", cache[0].Content)

	AppendChatCache(key, "assistant", "hi there", 5)
	cache = GetChatCache(key)
	require.Len(t, cache, 2)
	assert.Equal(t, "hi there", cache[1].Content)
}

func TestAppendChatCache_Truncation(t *testing.T) {
	setupTestDB(t)

	key := "test-truncation"
	limit := 3

	for i := 0; i < 5; i++ {
		AppendChatCache(key, "user", "msg", limit)
	}

	cache := GetChatCache(key)
	assert.Len(t, cache, limit)
}

func TestAppendChatCache_TruncationKeepsNewest(t *testing.T) {
	setupTestDB(t)

	key := "test-truncation-newest"
	limit := 3

	for i := 0; i < 5; i++ {
		AppendChatCache(key, "user", fmt.Sprintf("msg%d", i), limit)
	}

	cache := GetChatCache(key)
	require.Len(t, cache, limit)
	// Should keep the 3 newest: msg2, msg3, msg4
	assert.Equal(t, "msg2", cache[0].Content)
	assert.Equal(t, "msg3", cache[1].Content)
	assert.Equal(t, "msg4", cache[2].Content)
}

func TestGetChatCache_Empty(t *testing.T) {
	setupTestDB(t)

	cache := GetChatCache("nonexistent-key")
	assert.Nil(t, cache)
}

func TestDeleteChatCache(t *testing.T) {
	setupTestDB(t)

	key := "test-delete-cache"
	AppendChatCache(key, "user", "hello", 5)

	// Verify data exists before deletion
	cache := GetChatCache(key)
	require.NotNil(t, cache, "cache should exist before deletion")

	// DeleteChatCache should now correctly target the chat_history table
	result := DeleteChatCache(key)
	assert.True(t, result)

	// Verify data is actually gone from chat_history
	cache = GetChatCache(key)
	assert.Nil(t, cache, "cache should be nil after deletion")
}

func TestDeleteChatCache_IsolatesOtherKeys(t *testing.T) {
	setupTestDB(t)

	key1 := "test-delete-key-1"
	key2 := "test-delete-key-2"
	AppendChatCache(key1, "user", "hello", 5)
	AppendChatCache(key2, "user", "world", 5)

	result := DeleteChatCache(key1)
	assert.True(t, result)

	// key2 should still exist
	cache := GetChatCache(key2)
	require.NotNil(t, cache)
	assert.Equal(t, "world", cache[0].Content)

	// key1 should be gone
	cache = GetChatCache(key1)
	assert.Nil(t, cache)
}
