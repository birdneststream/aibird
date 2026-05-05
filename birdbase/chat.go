package birdbase

import (
	"encoding/json"
)

func PutChatHistory(key string, messages []Message) error {
	return Data.PutChatHistory(key, messages)
}

func (s *SQLiteDB) PutChatHistory(key string, messages []Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`
        INSERT INTO chat_history (cache_key, messages, expires_at, updated_at)
        VALUES (?, ?, datetime('now', '+24 hours'), datetime('now'))
        ON CONFLICT(cache_key) DO UPDATE SET
            messages = excluded.messages,
            expires_at = datetime('now', '+24 hours'),
            updated_at = datetime('now')
    `, key, messagesJSON)

	return err
}

func GetChatHistory(key string) ([]Message, error) {
	return Data.GetChatHistory(key)
}

func (s *SQLiteDB) GetChatHistory(key string) ([]Message, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var messagesJSON []byte
	err := s.db.QueryRow(`
        SELECT messages FROM chat_history 
        WHERE cache_key = ? AND expires_at > datetime('now')
    `, key).Scan(&messagesJSON)

	if err != nil {
		return nil, err
	}

	var messages []Message
	err = json.Unmarshal(messagesJSON, &messages)
	return messages, err
}

func IncrementUserUsage(ident, host string) (count int, shouldNag bool, err error) {
	return Data.IncrementUserUsage(ident, host)
}

func (s *SQLiteDB) IncrementUserUsage(ident, host string) (count int, shouldNag bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	err = s.db.QueryRow(`
        INSERT INTO user_usage (ident, host, total_uses, updated_at)
        VALUES (?, ?, 1, datetime('now'))
        ON CONFLICT(ident, host) DO UPDATE SET
            total_uses = total_uses + 1,
            updated_at = datetime('now')
        RETURNING total_uses
    `, ident, host).Scan(&count)

	if err != nil {
		return 0, false, err
	}

	return count, count > 0 && count%30 == 0, nil
}

// DeleteChatHistory removes a chat history entry by its cache key.
func DeleteChatHistory(key string) error {
	return Data.DeleteChatHistory(key)
}

func (s *SQLiteDB) DeleteChatHistory(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM chat_history WHERE cache_key = ?", key)
	return err
}
