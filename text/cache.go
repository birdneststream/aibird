package text

import (
	"aibird/birdbase"
	"aibird/logger"
	"encoding/json"
	"errors"

	"git.mills.io/prologic/bitcask"
)

func AppendChatCache(key string, whoIsTalking string, message string, contextLimit int) {
	// Start with an empty cache
	var cache []Message

	// If a cache already exists, get it
	if birdbase.Has(key) {
		cache = GetChatCache(key)
	}

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

	// Write the updated cache back to the database
	cacheBytes, err := json.Marshal(cache)
	if err != nil {
		logger.Error("Failed to marshal chat cache for appending", "key", key, "error", err)
		return
	}

	err = birdbase.PutBytesExpireHours(key, cacheBytes, 24)
	if err != nil {
		logger.Error("Failed to put appended chat cache", "key", key, "error", err)
	}
}

func GetChatCache(key string) []Message {
	data, err := birdbase.Get(key)
	if err != nil {
		// Use errors.Is for robust error checking, specifically for the key not found case.
		if !errors.Is(err, bitcask.ErrKeyNotFound) {
			logger.Error("Failed to get chat cache", "key", key, "error", err)
		}
		return nil
	}

	var messages []Message
	err = json.Unmarshal(data, &messages)
	if err != nil {
		logger.Error("Failed to unmarshal chat cache", "key", key, "error", err)
		return nil
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
	// If a cache already exists, get it
	if !birdbase.Has(key) {
		return
	}

	cache := GetChatCache(key)
	if len(cache) == 0 {
		return
	}

	// Remove the last message
	cache = cache[:len(cache)-1]

	// Write the updated cache back to the database
	cacheBytes, err := json.Marshal(cache)
	if err != nil {
		logger.Error("Failed to marshal chat cache for truncating", "key", key, "error", err)
		return
	}

	err = birdbase.PutBytesExpireHours(key, cacheBytes, 24)
	if err != nil {
		logger.Error("Failed to put truncated chat cache", "key", key, "error", err)
	}
}
