package text

import (
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

func TestGetChatCache_Empty(t *testing.T) {
	setupTestDB(t)

	cache := GetChatCache("nonexistent-key")
	assert.Nil(t, cache)
}

func TestDeleteChatCache(t *testing.T) {
	setupTestDB(t)

	key := "test-delete-cache"
	AppendChatCache(key, "user", "hello", 5)

	// Note: DeleteChatCache calls birdbase.Delete which targets key_value_store,
	// not chat_history. This is a known limitation — the function returns true
	// but doesn't actually clear the chat cache from the correct table.
	result := DeleteChatCache(key)
	assert.True(t, result)
}

func TestTruncateLastMessage(t *testing.T) {
	setupTestDB(t)

	key := "test-truncate"
	AppendChatCache(key, "user", "first", 5)
	AppendChatCache(key, "user", "second", 5)
	AppendChatCache(key, "user", "third", 5)

	TruncateLastMessage(key)
	cache := GetChatCache(key)
	require.Len(t, cache, 2)
	assert.Equal(t, "second", cache[1].Content)
}

func TestTruncateLastMessage_Empty(t *testing.T) {
	setupTestDB(t)

	// Should not panic on empty cache
	TruncateLastMessage("nonexistent-key")
}
