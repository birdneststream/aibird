package birdbase

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteChatHistory(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	key := "user:alice@irc#test"

	// Insert chat history
	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	err = db.PutChatHistory(key, msgs)
	require.NoError(t, err)

	// Verify it exists
	got, err := db.GetChatHistory(key)
	require.NoError(t, err)
	require.Len(t, got, 2)

	// Delete it
	err = db.DeleteChatHistory(key)
	require.NoError(t, err)

	// Verify it's gone
	got, err = db.GetChatHistory(key)
	assert.Nil(t, got)
	assert.Error(t, err, "expected error after deleting non-existent key")
}

func TestDeleteChatHistory_IsolatesOtherKeys(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	key1 := "user:alice@irc#test"
	key2 := "user:bob@irc#test"

	err = db.PutChatHistory(key1, []Message{{Role: "user", Content: "alice msg"}})
	require.NoError(t, err)
	err = db.PutChatHistory(key2, []Message{{Role: "user", Content: "bob msg"}})
	require.NoError(t, err)

	// Delete only key1
	err = db.DeleteChatHistory(key1)
	require.NoError(t, err)

	// key2 should still exist
	got, err := db.GetChatHistory(key2)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "bob msg", got[0].Content)

	// key1 should be gone
	got, _ = db.GetChatHistory(key1)
	assert.Nil(t, got)
}

func TestDeleteChatHistory_NonExistentKey(t *testing.T) {
	db, err := NewSQLiteDB(":memory:")
	require.NoError(t, err)
	defer db.db.Close()

	// Deleting a non-existent key should not error
	err = db.DeleteChatHistory("nonexistent-key")
	assert.NoError(t, err)
}
