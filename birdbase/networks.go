package birdbase

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"aibird/logger"
)

func SaveNetwork(networkName string, network *NetworkData) error {
	return Data.SaveNetwork(networkName, network)
}

func (s *SQLiteDB) SaveNetwork(networkName string, network *NetworkData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	networkID := int64(0)
	err = tx.QueryRow(`
		INSERT INTO networks (network_name, enabled, nick, user_name, real_name, preserve_modes, 
			ping_delay, version, throttle, burst, action_trigger, modes_at_once, 
			nickserv_pass, server_pass, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
		ON CONFLICT(network_name) DO UPDATE SET
			enabled = excluded.enabled,
			nick = excluded.nick,
			user_name = excluded.user_name,
			real_name = excluded.real_name,
			preserve_modes = excluded.preserve_modes,
			ping_delay = excluded.ping_delay,
			version = excluded.version,
			throttle = excluded.throttle,
			burst = excluded.burst,
			action_trigger = excluded.action_trigger,
			modes_at_once = excluded.modes_at_once,
			nickserv_pass = excluded.nickserv_pass,
			server_pass = excluded.server_pass,
			updated_at = datetime('now')
		RETURNING id
	`, networkName, network.Enabled, network.Nick, network.User, network.Name, network.PreserveModes,
		network.PingDelay, network.Version, network.Throttle, network.Burst, network.ActionTrigger,
		network.ModesAtOnce, network.NickServPass, network.Pass).Scan(&networkID)

	if err != nil {
		return err
	}

	if err := saveNetworkServers(tx, networkID, network.Servers); err != nil {
		return err
	}
	if err := saveNetworkAdminHosts(tx, networkID, network.AdminHosts); err != nil {
		return err
	}
	if err := saveNetworkChannels(tx, networkID, network.Channels); err != nil {
		return err
	}
	if err := saveNetworkIgnoredNicks(tx, networkID, network.IgnoredNicks); err != nil {
		return err
	}
	if err := saveNetworkDeniedCommands(tx, networkID, network.DenyCommands); err != nil {
		return err
	}

	return tx.Commit()
}

// saveNetworkServers persists server entries for a network.
func saveNetworkServers(tx *sql.Tx, networkID int64, servers []ServerData) error {
	if len(servers) > 0 {
		for _, server := range servers {
			_, err := tx.Exec(`
				INSERT INTO servers (network_id, host, port, ssl, skip_ssl_verify)
				VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(network_id, host, port) DO UPDATE SET
					ssl = excluded.ssl,
					skip_ssl_verify = excluded.skip_ssl_verify
			`, networkID, server.Host, server.Port, server.SSL, server.SkipSSLVerify)
			if err != nil {
				return err
			}
		}

		var serverHosts []interface{}
		for _, server := range servers {
			serverHosts = append(serverHosts, server.Host)
		}

		if err := syncSimpleCollection(tx, "servers", "host", "network_id", networkID, serverHosts); err != nil {
			return err
		}
	} else {
		_, err := tx.Exec("DELETE FROM servers WHERE network_id = ?", networkID)
		if err != nil {
			return err
		}
	}
	return nil
}

// saveNetworkAdminHosts persists admin host entries for a network.
func saveNetworkAdminHosts(tx *sql.Tx, networkID int64, admins []AdminHost) error {
	if len(admins) > 0 {
		for _, admin := range admins {
			_, err := tx.Exec(`
				INSERT INTO admin_hosts (network_id, host, ident, is_owner)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(network_id, host, ident) DO UPDATE SET
					is_owner = excluded.is_owner
			`, networkID, admin.Host, admin.Ident, admin.Owner)
			if err != nil {
				return err
			}
		}

		var hostIdentPairs []string
		var args []interface{}
		args = append(args, networkID)

		for _, admin := range admins {
			hostIdentPairs = append(hostIdentPairs, "(host = ? AND ident = ?)")
			args = append(args, admin.Host, admin.Ident)
		}

		deleteQuery := fmt.Sprintf(`
			DELETE FROM admin_hosts 
			WHERE network_id = ? AND NOT (%s)
		`, strings.Join(hostIdentPairs, " OR "))

		_, err := tx.Exec(deleteQuery, args...)
		if err != nil {
			return err
		}
	} else {
		_, err := tx.Exec("DELETE FROM admin_hosts WHERE network_id = ?", networkID)
		if err != nil {
			return err
		}
	}
	return nil
}

// saveNetworkChannels persists channel entries and their denied commands for a network.
func saveNetworkChannels(tx *sql.Tx, networkID int64, channels []ChannelData) error {
	if len(channels) > 0 {
		for _, channel := range channels {
			_, err := tx.Exec(`
				INSERT INTO channels (network_id, name, preserve_modes, ai_enabled, sd_enabled, 
					image_describe, sound_enabled, video_enabled, action_trigger, trim_output)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(network_id, name) DO UPDATE SET
					preserve_modes = excluded.preserve_modes,
					ai_enabled = excluded.ai_enabled,
					sd_enabled = excluded.sd_enabled,
					image_describe = excluded.image_describe,
					sound_enabled = excluded.sound_enabled,
					video_enabled = excluded.video_enabled,
					action_trigger = excluded.action_trigger,
					trim_output = excluded.trim_output,
					updated_at = datetime('now')
			`, networkID, channel.Name, channel.PreserveModes, channel.Ai, channel.Sd,
				channel.ImageDescribe, channel.Sound, channel.Video, channel.ActionTrigger, channel.TrimOutput)
			if err != nil {
				return err
			}

			channelID := int64(0)
			err = tx.QueryRow("SELECT id FROM channels WHERE network_id = ? AND name = ?", networkID, channel.Name).Scan(&channelID)
			if err != nil {
				return err
			}

			if err := saveChannelDeniedCommands(tx, channelID, channel.DenyCommands); err != nil {
				return err
			}
		}

		var channelNames []interface{}
		for _, channel := range channels {
			channelNames = append(channelNames, channel.Name)
		}

		if err := syncSimpleCollection(tx, "channels", "name", "network_id", networkID, channelNames); err != nil {
			return err
		}
	} else {
		_, err := tx.Exec("DELETE FROM channels WHERE network_id = ?", networkID)
		if err != nil {
			return err
		}
	}
	return nil
}

// saveChannelDeniedCommands persists denied commands for a single channel.
func saveChannelDeniedCommands(tx *sql.Tx, channelID int64, denyCommands []string) error {
	if len(denyCommands) > 0 {
		for _, cmd := range denyCommands {
			_, err := tx.Exec(`
				INSERT INTO denied_commands (network_id, channel_id, command)
				VALUES (NULL, ?, ?)
				ON CONFLICT(network_id, channel_id, command) DO NOTHING
			`, channelID, cmd)
			if err != nil {
				return err
			}
		}

		var chanCmds []interface{}
		for _, cmd := range denyCommands {
			chanCmds = append(chanCmds, cmd)
		}

		if err := syncSimpleCollection(tx, "denied_commands", "command", "channel_id", channelID, chanCmds); err != nil {
			return err
		}
	} else {
		_, err := tx.Exec("DELETE FROM denied_commands WHERE channel_id = ?", channelID)
		if err != nil {
			return err
		}
	}
	return nil
}

// saveNetworkIgnoredNicks persists ignored nick entries for a network.
func saveNetworkIgnoredNicks(tx *sql.Tx, networkID int64, nicks []string) error {
	if len(nicks) > 0 {
		for _, nick := range nicks {
			_, err := tx.Exec(`
				INSERT INTO ignored_nicks (network_id, nickname)
				VALUES (?, ?)
				ON CONFLICT(network_id, nickname) DO NOTHING
			`, networkID, nick)
			if err != nil {
				return err
			}
		}

		var nickInterfaces []interface{}
		for _, nick := range nicks {
			nickInterfaces = append(nickInterfaces, nick)
		}
		if err := syncSimpleCollection(tx, "ignored_nicks", "nickname", "network_id", networkID, nickInterfaces); err != nil {
			return err
		}
	} else {
		_, err := tx.Exec("DELETE FROM ignored_nicks WHERE network_id = ?", networkID)
		if err != nil {
			return err
		}
	}
	return nil
}

// saveNetworkDeniedCommands persists network-level denied commands.
func saveNetworkDeniedCommands(tx *sql.Tx, networkID int64, cmds []string) error {
	if len(cmds) > 0 {
		for _, cmd := range cmds {
			_, err := tx.Exec(`
				INSERT INTO denied_commands (network_id, channel_id, command)
				VALUES (?, NULL, ?)
				ON CONFLICT(network_id, channel_id, command) DO NOTHING
			`, networkID, cmd)
			if err != nil {
				return err
			}
		}

		var cmdInterfaces []interface{}
		for _, cmd := range cmds {
			cmdInterfaces = append(cmdInterfaces, cmd)
		}
		if err := syncSimpleCollection(tx, "denied_commands", "command", "network_id", networkID, cmdInterfaces); err != nil {
			return err
		}
	} else {
		_, err := tx.Exec("DELETE FROM denied_commands WHERE network_id = ?", networkID)
		if err != nil {
			return err
		}
	}
	return nil
}

func LoadNetwork(networkName string) (*NetworkData, error) {
	return Data.LoadNetwork(networkName)
}

func (s *SQLiteDB) LoadNetwork(networkName string) (*NetworkData, error) { //nolint:gocyclo // sequential DB loading with multiple table queries
	s.mu.RLock()
	defer s.mu.RUnlock()

	network := &NetworkData{}

	var networkID int64
	err := s.db.QueryRow(`
		SELECT id, enabled, nick, user_name, real_name, preserve_modes,
			ping_delay, version, throttle, burst, action_trigger, modes_at_once,
			nickserv_pass, server_pass
		FROM networks WHERE network_name = ?
	`, networkName).Scan(&networkID, &network.Enabled, &network.Nick, &network.User, &network.Name,
		&network.PreserveModes, &network.PingDelay, &network.Version, &network.Throttle,
		&network.Burst, &network.ActionTrigger, &network.ModesAtOnce, &network.NickServPass, &network.Pass)

	if err != nil {
		return nil, err
	}

	serverRows, err := s.db.Query(`
		SELECT host, port, ssl, skip_ssl_verify
		FROM servers WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer serverRows.Close()

	for serverRows.Next() {
		var server ServerData
		err = serverRows.Scan(&server.Host, &server.Port, &server.SSL, &server.SkipSSLVerify)
		if err != nil {
			return nil, err
		}
		network.Servers = append(network.Servers, server)
	}
	err = serverRows.Err()
	if err != nil {
		return nil, err
	}

	adminRows, err := s.db.Query(`
		SELECT host, ident, is_owner
		FROM admin_hosts WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer adminRows.Close()

	for adminRows.Next() {
		var admin AdminHost
		err = adminRows.Scan(&admin.Host, &admin.Ident, &admin.Owner)
		if err != nil {
			return nil, err
		}
		network.AdminHosts = append(network.AdminHosts, admin)
	}
	err = adminRows.Err()
	if err != nil {
		return nil, err
	}

	ignoredRows, err := s.db.Query(`
		SELECT nickname
		FROM ignored_nicks WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer ignoredRows.Close()

	for ignoredRows.Next() {
		var nick string
		err = ignoredRows.Scan(&nick)
		if err != nil {
			return nil, err
		}
		network.IgnoredNicks = append(network.IgnoredNicks, nick)
	}
	err = ignoredRows.Err()
	if err != nil {
		return nil, err
	}

	deniedRows, err := s.db.Query(`
		SELECT command
		FROM denied_commands WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer deniedRows.Close()

	for deniedRows.Next() {
		var cmd string
		err = deniedRows.Scan(&cmd)
		if err != nil {
			return nil, err
		}
		network.DenyCommands = append(network.DenyCommands, cmd)
	}
	err = deniedRows.Err()
	if err != nil {
		return nil, err
	}

	return network, nil
}

func LoadNetworkChannels(networkName string) ([]ChannelData, error) {
	return Data.LoadNetworkChannels(networkName)
}

func (s *SQLiteDB) LoadNetworkChannels(networkName string) ([]ChannelData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var networkID int64
	err := s.db.QueryRow("SELECT id FROM networks WHERE network_name = ?", networkName).Scan(&networkID)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT name, preserve_modes, ai_enabled, sd_enabled, image_describe, 
		       sound_enabled, video_enabled, action_trigger, trim_output
		FROM channels WHERE network_id = ?
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []ChannelData
	for rows.Next() {
		var ch ChannelData
		err = rows.Scan(&ch.Name, &ch.PreserveModes, &ch.Ai, &ch.Sd, &ch.ImageDescribe,
			&ch.Sound, &ch.Video, &ch.ActionTrigger, &ch.TrimOutput)
		if err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return channels, nil
}

func DeleteChannel(networkName, channelName string) error {
	return Data.DeleteChannel(networkName, channelName)
}

func (s *SQLiteDB) DeleteChannel(networkName, channelName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		DELETE FROM channels 
		WHERE id IN (
			SELECT c.id FROM channels c
			JOIN networks n ON c.network_id = n.id
			WHERE n.network_name = ? AND c.name = ?
		)
	`, networkName, channelName)

	if err == nil {
		rowsAffected, _ := result.RowsAffected()
		logger.Debug("Deleted channel", "network", networkName, "channel", channelName, "rows_affected", rowsAffected)
	}

	return err
}

func DeleteNetwork(networkName string) error {
	return Data.DeleteNetwork(networkName)
}

func (s *SQLiteDB) DeleteNetwork(networkName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`DELETE FROM networks WHERE network_name = ?`, networkName)

	if err == nil {
		rowsAffected, _ := result.RowsAffected()
		logger.Debug("Deleted network", "network", networkName, "rows_affected", rowsAffected)
	}

	return err
}

func GetAllNetworkNames() ([]string, error) {
	return Data.GetAllNetworkNames()
}

func (s *SQLiteDB) GetAllNetworkNames() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT network_name FROM networks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var networks []string
	for rows.Next() {
		var networkName string
		err = rows.Scan(&networkName)
		if err != nil {
			return nil, err
		}
		networks = append(networks, networkName)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return networks, nil
}

func SaveNetworkUsers(networkName string, users []UserData) error {
	return Data.SaveNetworkUsers(networkName, users)
}

func SaveNetworkUsersWithChannels(networkName string, users []UserData, channels []ChannelData) error {
	return Data.SaveNetworkUsersWithChannels(networkName, users, channels)
}

func (s *SQLiteDB) SaveNetworkUsersWithChannels(networkName string, users []UserData, channels []ChannelData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var networkID int64
	err = tx.QueryRow("SELECT id FROM networks WHERE network_name = ?", networkName).Scan(&networkID)
	if err == sql.ErrNoRows {
		err = tx.QueryRow(`
			INSERT INTO networks (network_name, enabled, nick) 
			VALUES (?, 1, 'placeholder') 
			RETURNING id
		`, networkName).Scan(&networkID)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return s.saveUsersToDatabase(tx, networkID, users)
}

func (s *SQLiteDB) SaveNetworkUsers(networkName string, users []UserData) error {
	return s.SaveNetworkUsersWithChannels(networkName, users, []ChannelData{})
}

// saveUserModes persists user mode data for a given mode type (preserved or current).
// When ensureChannel is true, it creates missing channel rows and returns errors on failure.
// When ensureChannel is false, it silently skips modes for channels not yet in the database.
func saveUserModes(tx *sql.Tx, userID, networkID int64, modes []UserModeData, modeType string, ensureChannel bool) error {
	for _, modeData := range modes {
		modesJSON, err := json.Marshal(modeData.Modes)
		if err != nil {
			return err
		}

		if ensureChannel {
			if _, err := tx.Exec(`INSERT INTO channels (network_id, name) VALUES (?, ?) ON CONFLICT(network_id, name) DO NOTHING`, networkID, modeData.Channel); err != nil {
				return err
			}
		}

		var channelID int64
		err = tx.QueryRow("SELECT id FROM channels WHERE network_id = ? AND name = ?", networkID, modeData.Channel).Scan(&channelID)
		if err != nil {
			if ensureChannel {
				return err
			}
			continue
		}

		if _, err := tx.Exec(`INSERT INTO user_modes (user_id, channel_id, mode_type, modes) VALUES (?, ?, ?, ?) ON CONFLICT(user_id, channel_id, mode_type) DO UPDATE SET modes = excluded.modes, updated_at = datetime('now')`, userID, channelID, modeType, modesJSON); err != nil {
			return err
		}
	}
	return nil
}

func SaveSingleUser(networkName, ident, host string, user *UserData) error {
	return Data.SaveSingleUser(networkName, ident, host, user)
}

func (s *SQLiteDB) SaveSingleUser(networkName, ident, host string, user *UserData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var networkID int64
	err = tx.QueryRow("SELECT id FROM networks WHERE network_name = ?", networkName).Scan(&networkID)
	if err != nil {
		return fmt.Errorf("network not found: %w", err)
	}

	var userID int64
	err = tx.QueryRow(`
		INSERT INTO irc_users (network_id, nickname, ident, host, first_seen, latest_activity,
			latest_chat, is_admin, is_owner, ignored, access_level, ai_service, ai_model,
			ai_base_prompt, ai_personality)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(network_id, ident, host) DO UPDATE SET
			nickname = excluded.nickname,
			latest_activity = excluded.latest_activity,
			latest_chat = excluded.latest_chat,
			is_admin = excluded.is_admin,
			is_owner = excluded.is_owner,
			ignored = excluded.ignored,
			access_level = excluded.access_level,
			ai_service = excluded.ai_service,
			ai_model = excluded.ai_model,
			ai_base_prompt = excluded.ai_base_prompt,
			ai_personality = excluded.ai_personality,
			updated_at = datetime('now')
		RETURNING id
	`, networkID, user.NickName, user.Ident, user.Host, user.FirstSeen, user.LatestActivity,
		user.LatestChat, user.IsAdmin, user.IsOwner, user.Ignored, user.AccessLevel,
		user.AiService, user.AiModel, user.AiBasePrompt, user.AiPersonality).Scan(&userID)

	if err != nil {
		return err
	}

	if err := saveUserModes(tx, userID, networkID, user.PreservedModes, "preserved", false); err != nil {
		return err
	}
	if err := saveUserModes(tx, userID, networkID, user.CurrentModes, "current", false); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SQLiteDB) saveUsersToDatabase(tx *sql.Tx, networkID int64, users []UserData) error { //nolint:gocyclo // dedup + upsert + mode saving is inherently complex
	logger.Debug("saveUsersToDatabase called", "network_id", networkID, "users_count", len(users))

	userMap := make(map[string]*UserData)
	for i := range users {
		key := fmt.Sprintf("%s@%s", users[i].Ident, users[i].Host)
		if existing, exists := userMap[key]; exists {
			logger.Warn("Duplicate user detected (same ident@host), keeping latest activity", "network_id", networkID, "index", i, "existing_nick", existing.NickName, "new_nick", users[i].NickName, "ident", users[i].Ident, "host", users[i].Host)
			if users[i].LatestActivity > existing.LatestActivity {
				logger.Debug("Keeping newer nick", "old_nick", existing.NickName, "new_nick", users[i].NickName, "ident", users[i].Ident, "host", users[i].Host)
				userMap[key] = &users[i]
			} else {
				logger.Debug("Keeping older nick", "kept_nick", existing.NickName, "discarded_nick", users[i].NickName, "ident", users[i].Ident, "host", users[i].Host)
			}
		} else {
			userMap[key] = &users[i]
		}
	}

	deduplicatedUsers := make([]UserData, 0, len(userMap))
	for _, user := range userMap {
		deduplicatedUsers = append(deduplicatedUsers, *user)
	}

	logger.Debug("Deduplicated users", "network_id", networkID, "original_count", len(users), "deduplicated_count", len(deduplicatedUsers))

	for i := range deduplicatedUsers {
		logger.Debug("Upserting user", "network_id", networkID, "index", i, "nickname", deduplicatedUsers[i].NickName, "ident", deduplicatedUsers[i].Ident, "host", deduplicatedUsers[i].Host)

		var userID int64
		err := tx.QueryRow(`
			INSERT INTO irc_users (network_id, nickname, ident, host, first_seen, latest_activity,
				latest_chat, is_admin, is_owner, ignored, access_level, ai_service, ai_model,
				ai_base_prompt, ai_personality)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(network_id, ident, host) DO UPDATE SET
				nickname = excluded.nickname,
				latest_activity = excluded.latest_activity,
				latest_chat = excluded.latest_chat,
				is_admin = excluded.is_admin,
				is_owner = excluded.is_owner,
				ignored = excluded.ignored,
				access_level = excluded.access_level,
				ai_service = excluded.ai_service,
				ai_model = excluded.ai_model,
				ai_base_prompt = excluded.ai_base_prompt,
				ai_personality = excluded.ai_personality,
				updated_at = datetime('now')
			RETURNING id
		`, networkID, deduplicatedUsers[i].NickName, deduplicatedUsers[i].Ident, deduplicatedUsers[i].Host, deduplicatedUsers[i].FirstSeen, deduplicatedUsers[i].LatestActivity,
			deduplicatedUsers[i].LatestChat, deduplicatedUsers[i].IsAdmin, deduplicatedUsers[i].IsOwner, deduplicatedUsers[i].Ignored, deduplicatedUsers[i].AccessLevel,
			deduplicatedUsers[i].AiService, deduplicatedUsers[i].AiModel, deduplicatedUsers[i].AiBasePrompt, deduplicatedUsers[i].AiPersonality).Scan(&userID)

		if err != nil {
			return err
		}

		if err := saveUserModes(tx, userID, networkID, deduplicatedUsers[i].PreservedModes, "preserved", true); err != nil {
			return err
		}
		if err := saveUserModes(tx, userID, networkID, deduplicatedUsers[i].CurrentModes, "current", true); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func LoadNetworkUsers(networkName string) ([]UserData, error) {
	return Data.LoadNetworkUsers(networkName)
}

func (s *SQLiteDB) LoadNetworkUsers(networkName string) ([]UserData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var networkID int64
	err := s.db.QueryRow("SELECT id FROM networks WHERE network_name = ?", networkName).Scan(&networkID)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(`
		SELECT u.id, u.nickname, u.ident, u.host, u.first_seen, u.latest_activity, u.latest_chat,
			u.is_admin, u.is_owner, u.ignored, u.access_level, u.ai_service, u.ai_model,
			u.ai_base_prompt, u.ai_personality,
			um.mode_type, um.modes, c.name as channel_name
		FROM irc_users u
		LEFT JOIN user_modes um ON um.user_id = u.id
		LEFT JOIN channels c ON um.channel_id = c.id
		WHERE u.network_id = ?
		ORDER BY u.id, um.mode_type
	`, networkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	userMap := make(map[int64]*UserData)

	for rows.Next() {
		var userID int64
		var scanned UserData
		var modeType sql.NullString
		var modesJSON []byte
		var channelName sql.NullString

		err = rows.Scan(&userID, &scanned.NickName, &scanned.Ident, &scanned.Host, &scanned.FirstSeen,
			&scanned.LatestActivity, &scanned.LatestChat, &scanned.IsAdmin, &scanned.IsOwner, &scanned.Ignored,
			&scanned.AccessLevel, &scanned.AiService, &scanned.AiModel, &scanned.AiBasePrompt, &scanned.AiPersonality,
			&modeType, &modesJSON, &channelName)
		if err != nil {
			return nil, err
		}

		// Get or create the map entry — always work via pointer to avoid
		// value-copy confusion and unnecessary re-allocations.
		ptr, exists := userMap[userID]
		if !exists {
			scanned.ID = int(userID)
			scanned.NetworkID = int(networkID)
			ptr = &scanned
			userMap[userID] = ptr
		}

		if modeType.Valid && modesJSON != nil {
			var modes []string
			if jsonErr := json.Unmarshal(modesJSON, &modes); jsonErr != nil {
				return nil, jsonErr
			}

			modeData := UserModeData{
				Channel: channelName.String,
				Modes:   modes,
			}

			if modeType.String == "preserved" {
				ptr.PreservedModes = append(ptr.PreservedModes, modeData)
			} else {
				ptr.CurrentModes = append(ptr.CurrentModes, modeData)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate users: %w", err)
	}

	users := make([]UserData, 0, len(userMap))
	for _, user := range userMap {
		users = append(users, *user)
	}

	return users, nil
}

func GetUserByIdentHost(networkName, ident, host string) (*UserData, error) {
	return Data.GetUserByIdentHost(networkName, ident, host)
}

func (s *SQLiteDB) GetUserByIdentHost(networkName, ident, host string) (*UserData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var user UserData
	err := s.db.QueryRow(`
		SELECT u.id, u.nickname, u.ident, u.host, u.first_seen, u.latest_activity, u.latest_chat,
			u.is_admin, u.is_owner, u.ignored, u.access_level, u.ai_service, u.ai_model,
			u.ai_base_prompt, u.ai_personality, n.id as network_id
		FROM irc_users u
		JOIN networks n ON u.network_id = n.id
		WHERE n.network_name = ? AND u.ident = ? AND u.host = ?
	`, networkName, ident, host).Scan(&user.ID, &user.NickName, &user.Ident, &user.Host,
		&user.FirstSeen, &user.LatestActivity, &user.LatestChat, &user.IsAdmin, &user.IsOwner,
		&user.Ignored, &user.AccessLevel, &user.AiService, &user.AiModel, &user.AiBasePrompt,
		&user.AiPersonality, &user.NetworkID)

	if err != nil {
		return nil, err
	}

	return &user, nil
}

func AddUserToChannel(networkName, ident, host, channelName string) error {
	return Data.AddUserToChannel(networkName, ident, host, channelName)
}

func (s *SQLiteDB) AddUserToChannel(networkName, ident, host, channelName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var userID, channelID int64

	err := s.db.QueryRow(`
		SELECT u.id, c.id 
		FROM irc_users u
		JOIN networks n ON u.network_id = n.id
		JOIN channels c ON c.network_id = n.id
		WHERE n.network_name = ? AND u.ident = ? AND u.host = ? AND c.name = ?
	`, networkName, ident, host, channelName).Scan(&userID, &channelID)

	if err != nil {
		logger.Debug("Cannot add user to channel - user or channel not found", "network", networkName, "ident", ident, "host", host, "channel", channelName, "error", err)
		return nil
	}

	_, err = s.db.Exec(`
		INSERT INTO user_channels (user_id, channel_id, joined_at)
		VALUES (?, ?, datetime('now'))
		ON CONFLICT(user_id, channel_id) DO NOTHING
	`, userID, channelID)

	if err == nil {
		logger.Debug("Added user to channel", "network", networkName, "ident", ident, "host", host, "channel", channelName)
	}

	return err
}

func RemoveUserFromChannel(networkName, ident, host, channelName string) error {
	return Data.RemoveUserFromChannel(networkName, ident, host, channelName)
}

func (s *SQLiteDB) RemoveUserFromChannel(networkName, ident, host, channelName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		DELETE FROM user_channels 
		WHERE user_id IN (
			SELECT u.id FROM irc_users u
			JOIN networks n ON u.network_id = n.id
			WHERE n.network_name = ? AND u.ident = ? AND u.host = ?
		) AND channel_id IN (
			SELECT c.id FROM channels c
			JOIN networks n ON c.network_id = n.id
			WHERE n.network_name = ? AND c.name = ?
		)
	`, networkName, ident, host, networkName, channelName)

	if err == nil {
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			logger.Debug("Removed user from channel", "network", networkName, "ident", ident, "host", host, "channel", channelName)
		}
	}

	return err
}

func RemoveUserFromAllChannels(networkName, ident, host string) error {
	return Data.RemoveUserFromAllChannels(networkName, ident, host)
}

func (s *SQLiteDB) RemoveUserFromAllChannels(networkName, ident, host string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.Exec(`
		DELETE FROM user_channels 
		WHERE user_id IN (
			SELECT u.id FROM irc_users u
			JOIN networks n ON u.network_id = n.id
			WHERE n.network_name = ? AND u.ident = ? AND u.host = ?
		)
	`, networkName, ident, host)

	if err == nil {
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected > 0 {
			logger.Debug("Removed user from all channels", "network", networkName, "ident", ident, "host", host, "channels_left", rowsAffected)
		}
	}

	return err
}

func UpdateUserActivity(networkName, ident, host string, activity int64, latestChat string) error {
	return Data.UpdateUserActivity(networkName, ident, host, activity, latestChat)
}

func (s *SQLiteDB) UpdateUserActivity(networkName, ident, host string, activity int64, latestChat string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		UPDATE irc_users SET 
			latest_activity = ?, 
			latest_chat = ?,
			updated_at = datetime('now')
		WHERE network_id = (SELECT id FROM networks WHERE network_name = ?)
			AND ident = ? AND host = ?
	`, activity, latestChat, networkName, ident, host)

	return err
}

// syncSimpleCollection deletes stale rows from table: it keeps only rows where
// parentIDCol matches parentID AND keyCol is in keepValues. If keepValues is empty,
// all rows matching parentID are deleted. The caller is responsible for upserting
// items before calling this.
//
// NOTE: table, keyCol, and parentIDCol MUST be compile-time string literals — they
// are interpolated directly into SQL via fmt.Sprintf and are NOT parameterized.
// keepValues are safely bound via "?" placeholders.
func syncSimpleCollection(tx *sql.Tx, table, keyCol, parentIDCol string, parentID int64, keepValues []interface{}) error {
	if len(keepValues) > 0 {
		placeholders := make([]string, len(keepValues))
		args := make([]interface{}, len(keepValues)+1)
		args[0] = parentID
		for i, val := range keepValues {
			placeholders[i] = "?"
			args[i+1] = val
		}

		deleteQuery := fmt.Sprintf(
			"DELETE FROM %s WHERE %s = ? AND %s NOT IN (%s)",
			table, parentIDCol, keyCol, strings.Join(placeholders, ","),
		)

		_, err := tx.Exec(deleteQuery, args...)
		return err
	}

	_, err := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s = ?", table, parentIDCol), parentID)
	return err
}
