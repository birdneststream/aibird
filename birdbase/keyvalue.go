package birdbase

import (
	"database/sql"
	"strconv"
	"time"
)

func Get(key string) ([]byte, error) {
	return Data.Get(key)
}

func (s *SQLiteDB) Get(key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var value []byte
	var expiresAt sql.NullTime

	err := s.db.QueryRow(`
        SELECT value_data, expires_at 
        FROM key_value_store 
        WHERE key_name = ? AND (expires_at IS NULL OR expires_at > datetime('now'))
    `, key).Scan(&value, &expiresAt)

	if err != nil {
		return nil, err
	}

	return value, nil
}

func Put(key string, value []byte) error {
	return Data.Put(key, value)
}

func (s *SQLiteDB) Put(key string, value []byte) error {
	return s.PutWithTTL(key, value, 0)
}

func PutWithTTL(key string, value []byte, ttl time.Duration) error {
	return Data.PutWithTTL(key, value, ttl)
}

func (s *SQLiteDB) PutWithTTL(key string, value []byte, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ttl > 0 {
		ttlSeconds := int64(ttl.Seconds())
		_, err := s.db.Exec(`
            INSERT INTO key_value_store (key_name, value_data, expires_at, updated_at)
            VALUES (?, ?, datetime('now', '+' || ? || ' seconds'), datetime('now'))
            ON CONFLICT(key_name) DO UPDATE SET
                value_data = excluded.value_data,
                expires_at = excluded.expires_at,
                updated_at = datetime('now')
        `, key, value, ttlSeconds)
		return err
	}

	_, err := s.db.Exec(`
        INSERT INTO key_value_store (key_name, value_data, expires_at, updated_at)
        VALUES (?, ?, NULL, datetime('now'))
        ON CONFLICT(key_name) DO UPDATE SET
            value_data = excluded.value_data,
            expires_at = excluded.expires_at,
            updated_at = datetime('now')
    `, key, value)

	return err
}

func Has(key string) bool {
	return Data.Has(key)
}

func (s *SQLiteDB) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var exists bool
	err := s.db.QueryRow(`
        SELECT 1 FROM key_value_store 
        WHERE key_name = ? AND (expires_at IS NULL OR expires_at > datetime('now'))
        LIMIT 1
    `, key).Scan(&exists)

	return err == nil
}

func Delete(key string) error {
	return Data.Delete(key)
}

func (s *SQLiteDB) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec("DELETE FROM key_value_store WHERE key_name = ?", key)
	return err
}

func PutString(key, value string) error {
	return Put(key, []byte(value))
}

func PutInt(key string, value int) error {
	return Put(key, []byte(strconv.Itoa(value)))
}

func PutBytes(key string, value []byte) error {
	return Put(key, value)
}

func PutBytesExpireHours(key string, value []byte, hours int) error {
	return PutWithTTL(key, value, time.Duration(hours)*time.Hour)
}

func PutStringExpireSeconds(key, value string, seconds int) error {
	return PutWithTTL(key, []byte(value), time.Duration(seconds)*time.Second)
}

func PutIntExpireHours(key string, value, hours int) error {
	return PutWithTTL(key, []byte(strconv.Itoa(value)), time.Duration(hours)*time.Hour)
}
