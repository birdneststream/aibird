package birdbase

func IncrementCommandUsage(network, nickname, command string) error {
	return Data.IncrementCommandUsage(network, nickname, command)
}

func (s *SQLiteDB) IncrementCommandUsage(network, nickname, command string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
        INSERT INTO command_leaderboard (network, nickname, command, count, updated_at)
        VALUES (?, ?, ?, 1, datetime('now'))
        ON CONFLICT(network, nickname, command) DO UPDATE SET
            count = count + 1,
            updated_at = datetime('now')
    `, network, nickname, command)

	return err
}

func GetNetworkLeaderboard(network string, limit int) ([]LeaderboardEntry, error) {
	return Data.GetNetworkLeaderboard(network, limit)
}

func (s *SQLiteDB) GetNetworkLeaderboard(network string, limit int) ([]LeaderboardEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
        SELECT nickname, command, count, updated_at
        FROM command_leaderboard 
        WHERE network = ?
        ORDER BY count DESC, updated_at DESC
        LIMIT ?
    `, network, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LeaderboardEntry
	for rows.Next() {
		var entry LeaderboardEntry
		err := rows.Scan(&entry.Nickname, &entry.Command, &entry.Count, &entry.UpdatedAt)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func GetNetworkUserTotals(network string, limit int) ([]UserTotalEntry, error) {
	return Data.GetNetworkUserTotals(network, limit)
}

func (s *SQLiteDB) GetNetworkUserTotals(network string, limit int) ([]UserTotalEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
        SELECT nickname, SUM(count) as total_count, MAX(updated_at) as latest_update
        FROM command_leaderboard 
        WHERE network = ?
        GROUP BY nickname
        ORDER BY total_count DESC, latest_update DESC
        LIMIT ?
    `, network, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []UserTotalEntry
	for rows.Next() {
		var entry UserTotalEntry
		err := rows.Scan(&entry.Nickname, &entry.TotalCount, &entry.UpdatedAt)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func GetGlobalLeaderboard(limit int) ([]GlobalLeaderboardEntry, error) {
	return Data.GetGlobalLeaderboard(limit)
}

func (s *SQLiteDB) GetGlobalLeaderboard(limit int) ([]GlobalLeaderboardEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
        SELECT network, nickname, command, count, updated_at
        FROM command_leaderboard 
        ORDER BY count DESC, updated_at DESC
        LIMIT ?
    `, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []GlobalLeaderboardEntry
	for rows.Next() {
		var entry GlobalLeaderboardEntry
		err := rows.Scan(&entry.Network, &entry.Nickname, &entry.Command, &entry.Count, &entry.UpdatedAt)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

func GetCommandLeaderboard(command string, limit int) ([]GlobalLeaderboardEntry, error) {
	return Data.GetCommandLeaderboard(command, limit)
}

func (s *SQLiteDB) GetCommandLeaderboard(command string, limit int) ([]GlobalLeaderboardEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
        SELECT network, nickname, command, count, updated_at
        FROM command_leaderboard 
        WHERE command = ?
        ORDER BY count DESC, updated_at DESC
        LIMIT ?
    `, command, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []GlobalLeaderboardEntry
	for rows.Next() {
		var entry GlobalLeaderboardEntry
		err := rows.Scan(&entry.Network, &entry.Nickname, &entry.Command, &entry.Count, &entry.UpdatedAt)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}
