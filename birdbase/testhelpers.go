package birdbase

// Test-only helpers. They are exported so packages under irc/ can drive the
// database layer without reaching into unexported fields, and they are named
// with the ForTesting suffix so they are never mistaken for production API.
//
// None of these start the async message worker; they write synchronously so
// tests observe deterministic state.

// CloseTestDB closes a database handle created by NewSQLiteDB.
func (s *SQLiteDB) CloseTestDB() error { return s.db.Close() }

// LimitToSingleConn pins a handle to one connection. Required for ":memory:"
// databases, where every connection would otherwise be a distinct database.
func (s *SQLiteDB) LimitToSingleConn() { s.db.SetMaxOpenConns(1) }

// ClearNetworkIDCache drops the cached network name → ID mappings. Each
// in-memory test database has its own ids, so a cache carried between tests
// would hand out an id belonging to a database that no longer exists.
func ClearNetworkIDCache() {
	networkIDCache.Range(func(key, _ any) bool {
		networkIDCache.Delete(key)
		return true
	})
}

// SeedNetworkForTesting inserts a minimal networks row and returns its id.
func SeedNetworkForTesting(networkName string) (int64, error) {
	if err := SaveNetwork(networkName, &NetworkData{}); err != nil {
		return 0, err
	}
	return resolveIDUncached(networkName), nil
}

// resolveIDUncached reads a network id straight from the table, skipping the
// cache so freshly created test rows are found.
func resolveIDUncached(networkName string) int64 {
	Data.mu.RLock()
	defer Data.mu.RUnlock()

	var id int64
	if err := Data.db.QueryRow(
		"SELECT id FROM networks WHERE network_name = ?", networkName,
	).Scan(&id); err != nil {
		return 0
	}
	return id
}

// SeedChannelMessageForTesting inserts one channel_messages row with an
// explicit timestamp and event type, bypassing the batched writer.
func SeedChannelMessageForTesting(networkID int64, channelName, nickname, eventType, message string, timestamp int64) error {
	Data.mu.Lock()
	defer Data.mu.Unlock()

	_, err := Data.db.Exec(`
		INSERT INTO channel_messages (network_id, channel_name, nickname, event_type, message, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)
	`, networkID, channelName, nickname, eventType, message, timestamp)
	return err
}

// SeedIRCUserForTesting inserts one irc_users row and returns its id.
func SeedIRCUserForTesting(networkID int64, nickname, ident, host string, firstSeen, latestActivity int64) (int64, error) {
	Data.mu.Lock()
	defer Data.mu.Unlock()

	var id int64
	err := Data.db.QueryRow(`
		INSERT INTO irc_users (network_id, nickname, ident, host, first_seen, latest_activity,
			latest_chat, is_admin, is_owner, ignored, access_level)
		VALUES (?, ?, ?, ?, ?, ?, '', 0, 0, 0, 0)
		RETURNING id
	`, networkID, nickname, ident, host, firstSeen, latestActivity).Scan(&id)
	return id, err
}

// ReadUserLatestActivityForTesting returns the stored latest_activity for an
// ident@host pair, or -1 when no row exists.
func ReadUserLatestActivityForTesting(networkID int64, ident, host string) (int64, error) {
	Data.mu.RLock()
	defer Data.mu.RUnlock()

	var ts int64
	err := Data.db.QueryRow(`
		SELECT latest_activity FROM irc_users
		WHERE network_id = ? AND ident = ? AND host = ?
	`, networkID, ident, host).Scan(&ts)
	if err != nil {
		return -1, err
	}
	return ts, nil
}
