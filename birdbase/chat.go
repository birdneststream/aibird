package birdbase

import (
	"database/sql"
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

func IncrementUserUsage(ident, host string) (int, bool, error) {
	return Data.IncrementUserUsage(ident, host)
}

func (s *SQLiteDB) IncrementUserUsage(ident, host string) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
        INSERT INTO user_usage (ident, host, total_uses, updated_at)
        VALUES (?, ?, 1, datetime('now'))
        ON CONFLICT(ident, host) DO UPDATE SET
            total_uses = total_uses + 1,
            updated_at = datetime('now')
    `, ident, host)

	if err != nil {
		return 0, false, err
	}

	var totalUses int
	err = s.db.QueryRow(`
        SELECT total_uses FROM user_usage WHERE ident = ? AND host = ?
    `, ident, host).Scan(&totalUses)

	if err != nil {
		return 0, false, err
	}

	shouldNag := totalUses > 0 && totalUses%30 == 0

	return totalUses, shouldNag, nil
}

func GetUserUsage(ident, host string) (int, error) {
	return Data.GetUserUsage(ident, host)
}

func (s *SQLiteDB) GetUserUsage(ident, host string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var totalUses int
	err := s.db.QueryRow(`
        SELECT total_uses FROM user_usage WHERE ident = ? AND host = ?
    `, ident, host).Scan(&totalUses)

	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}

	return totalUses, nil
}
