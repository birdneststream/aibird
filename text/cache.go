package text

import (
	"aibird/birdbase"
	"aibird/logger"
	"database/sql"
)

func AppendChatCache(key string, whoIsTalking string, message string, contextLimit int) {
	// Start with an empty cache
	var cache []Message

	// If a cache already exists, get it
	cache = GetChatCache(key)

	// Append the new message
	newMessage := Message{
		Role:    whoIsTalking,
		Content: message,
	}
	cache = append(cache, newMessage)

	// Truncate if the cache is too long
	if len(cache) > contextLimit {
		cache = cache[1:] // Remove the oldest message
	}

	// Convert to birdbase.Message for storage
	birdbaseCache := make([]birdbase.Message, len(cache))
	for i, msg := range cache {
		birdbaseCache[i] = birdbase.Message{Role: msg.Role, Content: msg.Content}
	}

	// Write the updated cache back to the database using specialized method
	err := birdbase.PutChatHistory(key, birdbaseCache)
	if err != nil {
		logger.Error("Failed to put appended chat cache", "key", key, "error", err)
	}
}

func GetChatCache(key string) []Message {
	birdbaseMessages, err := birdbase.GetChatHistory(key)
	if err != nil {
		// Use errors.Is for robust error checking, specifically for the key not found case.
		if err != sql.ErrNoRows {
			logger.Error("Failed to get chat cache", "key", key, "error", err)
		}
		return nil
	}

	// Convert from birdbase.Message to text.Message
	messages := make([]Message, len(birdbaseMessages))
	for i, msg := range birdbaseMessages {
		messages[i] = Message{Role: msg.Role, Content: msg.Content}
	}

	return messages
}

func DeleteChatCache(key string) bool {
	err := birdbase.Delete(key)
	if err != nil {
		logger.Error("Failed to delete chat cache", "key", key, "error", err)
		return false
	}

	return true
}

func TruncateLastMessage(key string) {
	// Get the existing cache
	cache := GetChatCache(key)
	if len(cache) == 0 {
		return
	}

	// Remove the last message
	cache = cache[:len(cache)-1]

	// Convert to birdbase.Message for storage
	birdbaseCache := make([]birdbase.Message, len(cache))
	for i, msg := range cache {
		birdbaseCache[i] = birdbase.Message{Role: msg.Role, Content: msg.Content}
	}

	// Write the updated cache back to the database using specialized method
	err := birdbase.PutChatHistory(key, birdbaseCache)
	if err != nil {
		logger.Error("Failed to put truncated chat cache", "key", key, "error", err)
	}
}
