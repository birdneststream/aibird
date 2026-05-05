// Package text provides text generation caching and shared text utilities.
package text

import (
	"database/sql"

	"aibird/birdbase"
	"aibird/logger"
)

func AppendChatCache(key, whoIsTalking, message string, contextLimit int) {
	// Start with an empty cache
	cache := GetChatCache(key)

	// Append the new message
	cache = append(cache, birdbase.Message{
		Role:    whoIsTalking,
		Content: message,
	})

	// Truncate if the cache is too long
	if len(cache) > contextLimit {
		cache = cache[len(cache)-contextLimit:]
	}

	// Write the updated cache back to the database
	err := birdbase.PutChatHistory(key, cache)
	if err != nil {
		logger.Error("Failed to put appended chat cache", "key", key, "error", err)
	}
}

func GetChatCache(key string) []birdbase.Message {
	cache, err := birdbase.GetChatHistory(key)
	if err != nil {
		if err != sql.ErrNoRows {
			logger.Error("Failed to get chat cache", "key", key, "error", err)
		}
		return nil
	}

	return cache
}

func DeleteChatCache(key string) bool {
	err := birdbase.DeleteChatHistory(key)
	if err != nil {
		logger.Error("Failed to delete chat cache", "key", key, "error", err)
		return false
	}

	return true
}
