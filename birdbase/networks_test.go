package birdbase

import (
	"testing"

	"aibird/logger"
)

func init() {
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})
}

// helperNewTestDB creates an in-memory SQLite database for testing.
func helperNewTestDB(t *testing.T) *SQLiteDB {
	t.Helper()
	db, err := NewSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	return db
}

// TestSaveAndLoadNetwork_RoundTrip verifies that a network can be saved and loaded back intact.
func TestSaveAndLoadNetwork_RoundTrip(t *testing.T) {
	db := helperNewTestDB(t)
	defer db.db.Close()

	network := &NetworkData{
		Enabled:       true,
		Nick:          "testbot",
		User:          "testuser",
		Name:          "Test Real Name",
		Pass:          "serverpass",
		PreserveModes: true,
		NickServPass:  "nickservpass",
		PingDelay:     30,
		Version:       "v1.0",
		Throttle:      300,
		Burst:         5,
		ActionTrigger: "!",
		ModesAtOnce:   4,
		Servers: []ServerData{
			{Host: "irc.example.com", Port: 6697, SSL: true, SkipSSLVerify: false},
			{Host: "irc2.example.com", Port: 6667, SSL: false, SkipSSLVerify: false},
		},
		AdminHosts: []AdminHost{
			{Host: "admin.example.com", Ident: "adminident", Owner: true},
			{Host: "op.example.com", Ident: "opident", Owner: false},
		},
		Channels: []ChannelData{
			{
				Name: "#test", PreserveModes: true, Ai: true, Sd: true,
				ImageDescribe: false, Sound: false, Video: false,
				ActionTrigger: "", TrimOutput: false, DenyCommands: []string{"debug"},
			},
			{
				Name: "#dev", PreserveModes: false, Ai: false, Sd: false,
				ImageDescribe: false, Sound: false, Video: false,
				ActionTrigger: "~", TrimOutput: true, DenyCommands: nil,
			},
		},
		IgnoredNicks: []string{"spambot", "flooduser"},
		DenyCommands: []string{"adminonlycmd"},
	}

	// Save
	err := db.SaveNetwork("testnet", network)
	if err != nil {
		t.Fatalf("SaveNetwork failed: %v", err)
	}

	// Load
	loaded, err := db.LoadNetwork("testnet")
	if err != nil {
		t.Fatalf("LoadNetwork failed: %v", err)
	}

	// Verify core fields
	if loaded.Nick != "testbot" {
		t.Errorf("Nick mismatch: got %q, want %q", loaded.Nick, "testbot")
	}
	if loaded.User != "testuser" {
		t.Errorf("User mismatch: got %q, want %q", loaded.User, "testuser")
	}
	if loaded.Name != "Test Real Name" {
		t.Errorf("Name mismatch: got %q, want %q", loaded.Name, "Test Real Name")
	}
	if !loaded.Enabled {
		t.Error("Enabled should be true")
	}
	if !loaded.PreserveModes {
		t.Error("PreserveModes should be true")
	}
	if loaded.PingDelay != 30 {
		t.Errorf("PingDelay mismatch: got %d, want 30", loaded.PingDelay)
	}
	if loaded.Throttle != 300 {
		t.Errorf("Throttle mismatch: got %d, want 300", loaded.Throttle)
	}
	if loaded.Burst != 5 {
		t.Errorf("Burst mismatch: got %d, want 5", loaded.Burst)
	}
	if loaded.ActionTrigger != "!" {
		t.Errorf("ActionTrigger mismatch: got %q, want %q", loaded.ActionTrigger, "!")
	}
	if loaded.ModesAtOnce != 4 {
		t.Errorf("ModesAtOnce mismatch: got %d, want 4", loaded.ModesAtOnce)
	}

	// Verify servers (order may vary from SQLite, so check by content)
	if len(loaded.Servers) != 2 {
		t.Fatalf("Expected 2 servers, got %d", len(loaded.Servers))
	}
	serverMap := make(map[string]ServerData)
	for _, s := range loaded.Servers {
		serverMap[s.Host] = s
	}
	if s, ok := serverMap["irc.example.com"]; !ok {
		t.Error("Missing server irc.example.com")
	} else if s.Port != 6697 || !s.SSL {
		t.Errorf("Server irc.example.com mismatch: %+v", s)
	}
	if s, ok := serverMap["irc2.example.com"]; !ok {
		t.Error("Missing server irc2.example.com")
	} else if s.Port != 6667 || s.SSL {
		t.Errorf("Server irc2.example.com mismatch: %+v", s)
	}

	// Verify admin hosts
	if len(loaded.AdminHosts) != 2 {
		t.Fatalf("Expected 2 admin hosts, got %d", len(loaded.AdminHosts))
	}
	adminMap := make(map[string]AdminHost)
	for _, a := range loaded.AdminHosts {
		adminMap[a.Host] = a
	}
	if a, ok := adminMap["admin.example.com"]; !ok {
		t.Error("Missing admin host admin.example.com")
	} else if !a.Owner {
		t.Error("admin.example.com should be owner")
	}
	if a, ok := adminMap["op.example.com"]; !ok {
		t.Error("Missing admin host op.example.com")
	} else if a.Owner {
		t.Error("op.example.com should not be owner")
	}

	// Verify ignored nicks
	if len(loaded.IgnoredNicks) != 2 {
		t.Fatalf("Expected 2 ignored nicks, got %d", len(loaded.IgnoredNicks))
	}
	nickMap := make(map[string]bool)
	for _, n := range loaded.IgnoredNicks {
		nickMap[n] = true
	}
	if !nickMap["spambot"] {
		t.Error("Missing ignored nick 'spambot'")
	}
	if !nickMap["flooduser"] {
		t.Error("Missing ignored nick 'flooduser'")
	}

	// Verify denied commands
	if len(loaded.DenyCommands) != 1 {
		t.Fatalf("Expected 1 denied command, got %d", len(loaded.DenyCommands))
	}
	if loaded.DenyCommands[0] != "adminonlycmd" {
		t.Errorf("Denied command mismatch: got %q, want %q", loaded.DenyCommands[0], "adminonlycmd")
	}
}

// TestSaveAndLoadNetwork_Channels verifies channel data survives round-trip.
func TestSaveAndLoadNetwork_Channels(t *testing.T) {
	db := helperNewTestDB(t)
	defer db.db.Close()

	network := &NetworkData{
		Enabled: true,
		Nick:    "bot",
		User:    "botuser",
		Name:    "Bot",
		Channels: []ChannelData{
			{
				Name: "#general", Ai: true, Sd: true, Sound: true, Video: true,
				PreserveModes: true, ImageDescribe: true, TrimOutput: false,
				ActionTrigger: "!",
			},
		},
	}

	err := db.SaveNetwork("chantest", network)
	if err != nil {
		t.Fatalf("SaveNetwork failed: %v", err)
	}

	// Load channels separately
	channels, err := db.LoadNetworkChannels("chantest")
	if err != nil {
		t.Fatalf("LoadNetworkChannels failed: %v", err)
	}

	if len(channels) != 1 {
		t.Fatalf("Expected 1 channel, got %d", len(channels))
	}
	ch := channels[0]
	if ch.Name != "#general" {
		t.Errorf("Channel name mismatch: got %q", ch.Name)
	}
	if !ch.Ai {
		t.Error("Channel Ai should be true")
	}
	if !ch.Sd {
		t.Error("Channel Sd should be true")
	}
}

// TestSaveNetwork_UpdateExisting verifies that saving the same network twice updates it.
func TestSaveNetwork_UpdateExisting(t *testing.T) {
	db := helperNewTestDB(t)
	defer db.db.Close()

	network1 := &NetworkData{
		Enabled: true, Nick: "bot1", User: "u", Name: "n",
		Servers: []ServerData{{Host: "irc.old.com", Port: 6667, SSL: false}},
	}

	err := db.SaveNetwork("updatetest", network1)
	if err != nil {
		t.Fatalf("First SaveNetwork failed: %v", err)
	}

	network2 := &NetworkData{
		Enabled: true, Nick: "bot2", User: "u", Name: "n",
		Servers: []ServerData{{Host: "irc.new.com", Port: 6697, SSL: true}},
	}

	err = db.SaveNetwork("updatetest", network2)
	if err != nil {
		t.Fatalf("Second SaveNetwork failed: %v", err)
	}

	loaded, err := db.LoadNetwork("updatetest")
	if err != nil {
		t.Fatalf("LoadNetwork failed: %v", err)
	}

	if loaded.Nick != "bot2" {
		t.Errorf("Nick should be updated to bot2, got %q", loaded.Nick)
	}

	if len(loaded.Servers) != 1 {
		t.Fatalf("Expected 1 server, got %d", len(loaded.Servers))
	}
	if loaded.Servers[0].Host != "irc.new.com" {
		t.Errorf("Server should be updated to irc.new.com, got %q", loaded.Servers[0].Host)
	}
}

// TestLoadNetwork_NotFound verifies that loading a non-existent network returns an error.
func TestLoadNetwork_NotFound(t *testing.T) {
	db := helperNewTestDB(t)
	defer db.db.Close()

	_, err := db.LoadNetwork("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent network")
	}
}

// TestDeleteNetwork verifies network deletion.
func TestDeleteNetwork(t *testing.T) {
	db := helperNewTestDB(t)
	defer db.db.Close()

	network := &NetworkData{Enabled: true, Nick: "bot", User: "u", Name: "n"}
	err := db.SaveNetwork("deletetest", network)
	if err != nil {
		t.Fatalf("SaveNetwork failed: %v", err)
	}

	err = db.DeleteNetwork("deletetest")
	if err != nil {
		t.Fatalf("DeleteNetwork failed: %v", err)
	}

	_, err = db.LoadNetwork("deletetest")
	if err == nil {
		t.Error("Expected error after deleting network")
	}
}

// TestGetAllNetworkNames verifies listing all saved network names.
func TestGetAllNetworkNames(t *testing.T) {
	db := helperNewTestDB(t)
	defer db.db.Close()

	// Initially empty
	names, err := db.GetAllNetworkNames()
	if err != nil {
		t.Fatalf("GetAllNetworkNames failed: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("Expected 0 networks, got %d", len(names))
	}

	// Save two networks
	n1 := &NetworkData{Enabled: true, Nick: "bot1", User: "u", Name: "n"}
	n2 := &NetworkData{Enabled: true, Nick: "bot2", User: "u", Name: "n"}
	if err := db.SaveNetwork("net1", n1); err != nil {
		t.Fatalf("SaveNetwork net1 failed: %v", err)
	}
	if err := db.SaveNetwork("net2", n2); err != nil {
		t.Fatalf("SaveNetwork net2 failed: %v", err)
	}

	names, err = db.GetAllNetworkNames()
	if err != nil {
		t.Fatalf("GetAllNetworkNames failed: %v", err)
	}

	nameMap := make(map[string]bool)
	for _, n := range names {
		nameMap[n] = true
	}
	if !nameMap["net1"] || !nameMap["net2"] {
		t.Errorf("Expected net1 and net2, got %v", names)
	}
}

// TestSaveNetworkUsersAndLoad verifies saving and loading users with modes.
func TestSaveNetworkUsersAndLoad(t *testing.T) {
	db := helperNewTestDB(t)
	defer db.db.Close()

	// First save a network with a channel (user modes reference channels)
	network := &NetworkData{
		Enabled: true, Nick: "bot", User: "u", Name: "n",
		Channels: []ChannelData{
			{Name: "#test", Ai: true, Sd: false},
		},
	}
	err := db.SaveNetwork("usertest", network)
	if err != nil {
		t.Fatalf("SaveNetwork failed: %v", err)
	}

	users := []UserData{
		{
			NickName: "alice", Ident: "alice", Host: "host1.com",
			FirstSeen: 1000000, LatestActivity: 2000000,
			LatestChat: "hello", IsAdmin: true, IsOwner: false,
			Ignored: false, AccessLevel: 5, AiService: "llamacpp",
			AiModel: "llama3", AiBasePrompt: "You are helpful", AiPersonality: "friendly",
			PreservedModes: []UserModeData{
				{Channel: "#test", Modes: []string{"o", "v"}},
			},
			CurrentModes: []UserModeData{
				{Channel: "#test", Modes: []string{"o"}},
			},
		},
		{
			NickName: "bob", Ident: "bob", Host: "host2.com",
			FirstSeen: 1100000, LatestActivity: 2100000,
			LatestChat: "hi there", IsAdmin: false, IsOwner: false,
			Ignored: false, AccessLevel: 0, AiService: "llamacpp",
		},
	}

	err = db.SaveNetworkUsersWithChannels("usertest", users, network.Channels)
	if err != nil {
		t.Fatalf("SaveNetworkUsersWithChannels failed: %v", err)
	}

	loaded, err := db.LoadNetworkUsers("usertest")
	if err != nil {
		t.Fatalf("LoadNetworkUsers failed: %v", err)
	}

	userMap := make(map[string]*UserData)
	for i := range loaded {
		userMap[loaded[i].NickName] = &loaded[i]
	}

	// Verify alice
	alice, ok := userMap["alice"]
	if !ok {
		t.Fatal("Missing user alice")
	}
	if alice.Ident != "alice" || alice.Host != "host1.com" {
		t.Errorf("alice ident/host mismatch: %s@%s", alice.Ident, alice.Host)
	}
	if !alice.IsAdmin {
		t.Error("alice should be admin")
	}
	if alice.AccessLevel != 5 {
		t.Errorf("alice AccessLevel should be 5, got %d", alice.AccessLevel)
	}
	if alice.AiModel != "llama3" {
		t.Errorf("alice AiModel should be llama3, got %q", alice.AiModel)
	}
	if len(alice.PreservedModes) != 1 || alice.PreservedModes[0].Channel != "#test" {
		t.Errorf("alice PreservedModes mismatch: %+v", alice.PreservedModes)
	}
	if len(alice.PreservedModes[0].Modes) != 2 {
		t.Errorf("alice preserved modes count: got %d, want 2", len(alice.PreservedModes[0].Modes))
	}
	if len(alice.CurrentModes) != 1 || alice.CurrentModes[0].Channel != "#test" {
		t.Errorf("alice CurrentModes mismatch: %+v", alice.CurrentModes)
	}

	// Verify bob
	bob, ok := userMap["bob"]
	if !ok {
		t.Fatal("Missing user bob")
	}
	if bob.IsAdmin {
		t.Error("bob should not be admin")
	}
}

// TestSaveSingleUser verifies individual user save/update.
func TestSaveSingleUser(t *testing.T) {
	db := helperNewTestDB(t)
	defer db.db.Close()

	// Setup: save network with channel
	network := &NetworkData{
		Enabled: true, Nick: "bot", User: "u", Name: "n",
		Channels: []ChannelData{{Name: "#test"}},
	}
	err := db.SaveNetwork("singleuser", network)
	if err != nil {
		t.Fatalf("SaveNetwork failed: %v", err)
	}

	user := &UserData{
		NickName: "charlie", Ident: "charlie", Host: "host.com",
		FirstSeen: 1000, LatestActivity: 2000,
		IsAdmin: false, AiService: "llamacpp",
	}

	err = db.SaveSingleUser("singleuser", "charlie", "host.com", user)
	if err != nil {
		t.Fatalf("SaveSingleUser failed: %v", err)
	}

	// Load and verify
	loaded, err := db.GetUserByIdentHost("singleuser", "charlie", "host.com")
	if err != nil {
		t.Fatalf("GetUserByIdentHost failed: %v", err)
	}
	if loaded.NickName != "charlie" {
		t.Errorf("Expected nickname charlie, got %q", loaded.NickName)
	}

	// Update the user
	user.IsAdmin = true
	user.LatestActivity = 3000
	err = db.SaveSingleUser("singleuser", "charlie", "host.com", user)
	if err != nil {
		t.Fatalf("Second SaveSingleUser failed: %v", err)
	}

	updated, err := db.GetUserByIdentHost("singleuser", "charlie", "host.com")
	if err != nil {
		t.Fatalf("GetUserByIdentHost after update failed: %v", err)
	}
	if !updated.IsAdmin {
		t.Error("User should be admin after update")
	}
}

// TestDeleteChannel verifies channel deletion.
func TestDeleteChannel(t *testing.T) {
	db := helperNewTestDB(t)
	defer db.db.Close()

	network := &NetworkData{
		Enabled: true, Nick: "bot", User: "u", Name: "n",
		Channels: []ChannelData{
			{Name: "#keep"},
			{Name: "#delete"},
		},
	}
	err := db.SaveNetwork("chandel", network)
	if err != nil {
		t.Fatalf("SaveNetwork failed: %v", err)
	}

	err = db.DeleteChannel("chandel", "#delete")
	if err != nil {
		t.Fatalf("DeleteChannel failed: %v", err)
	}

	channels, err := db.LoadNetworkChannels("chandel")
	if err != nil {
		t.Fatalf("LoadNetworkChannels failed: %v", err)
	}

	for _, ch := range channels {
		if ch.Name == "#delete" {
			t.Error("Channel #delete should have been deleted")
		}
	}
}

// TestLoadNetworkUsers_MultipleModes verifies that a user with preserved and
// current modes across multiple channels round-trips correctly through
// SaveNetworkUsers → LoadNetworkUsers. This is a regression test for a
// previous value-copy bug where modes could be silently lost when the same
// user appeared in multiple LEFT JOIN rows.
func TestLoadNetworkUsers_MultipleModes(t *testing.T) {
	db := helperNewTestDB(t)
	defer db.db.Close()

	// Create a network with two channels
	network := &NetworkData{
		Enabled: true, Nick: "bot", User: "bot", Name: "Bot",
		Channels: []ChannelData{
			{Name: "#chan1", PreserveModes: true},
			{Name: "#chan2", PreserveModes: true},
		},
	}
	if err := db.SaveNetwork("modetest", network); err != nil {
		t.Fatalf("SaveNetwork: %v", err)
	}

	users := []UserData{
		{
			NickName: "alice", Ident: "~alice", Host: "host.alice",
			FirstSeen: 1000, LatestActivity: 2000, AiService: "llamacpp",
			PreservedModes: []UserModeData{
				{Channel: "#chan1", Modes: []string{"o", "v"}},
				{Channel: "#chan2", Modes: []string{"v"}},
			},
			CurrentModes: []UserModeData{
				{Channel: "#chan1", Modes: []string{"o"}},
				{Channel: "#chan2", Modes: []string{"v"}},
			},
		},
		{
			NickName: "bob", Ident: "~bob", Host: "host.bob",
			FirstSeen: 3000, LatestActivity: 4000, AiService: "llamacpp",
			// bob has no modes at all
			PreservedModes: nil,
			CurrentModes:   nil,
		},
		{
			NickName: "carol", Ident: "~carol", Host: "host.carol",
			FirstSeen: 5000, LatestActivity: 6000, AiService: "glm",
			PreservedModes: []UserModeData{
				{Channel: "#chan1", Modes: []string{"v"}},
			},
			CurrentModes: []UserModeData{
				{Channel: "#chan1", Modes: []string{"v"}},
			},
		},
	}

	if err := db.SaveNetworkUsers("modetest", users); err != nil {
		t.Fatalf("SaveNetworkUsers: %v", err)
	}

	loaded, err := db.LoadNetworkUsers("modetest")
	if err != nil {
		t.Fatalf("LoadNetworkUsers: %v", err)
	}

	// Build a lookup map by ident for easier assertions
	got := make(map[string]*UserData)
	for i := range loaded {
		got[loaded[i].Ident] = &loaded[i]
	}

	// Alice should have all modes preserved
	alice, ok := got["~alice"]
	if !ok {
		t.Fatal("alice not found in loaded users")
	}
	if len(alice.PreservedModes) != 2 {
		t.Errorf("alice PreservedModes: got %d entries, want 2", len(alice.PreservedModes))
	}
	if len(alice.CurrentModes) != 2 {
		t.Errorf("alice CurrentModes: got %d entries, want 2", len(alice.CurrentModes))
	}
	assertHasMode := func(modes []UserModeData, channel string, expected []string) {
		t.Helper()
		for _, m := range modes {
			if m.Channel == channel {
				if len(m.Modes) != len(expected) {
					t.Errorf("modes for %s: got %v, want %v", channel, m.Modes, expected)
				}
				return
			}
		}
		t.Errorf("no modes found for channel %s", channel)
	}
	assertHasMode(alice.PreservedModes, "#chan1", []string{"o", "v"})
	assertHasMode(alice.PreservedModes, "#chan2", []string{"v"})
	assertHasMode(alice.CurrentModes, "#chan1", []string{"o"})
	assertHasMode(alice.CurrentModes, "#chan2", []string{"v"})

	// Bob should have zero modes
	bob, ok := got["~bob"]
	if !ok {
		t.Fatal("bob not found in loaded users")
	}
	if len(bob.PreservedModes) != 0 {
		t.Errorf("bob PreservedModes: got %d, want 0", len(bob.PreservedModes))
	}
	if len(bob.CurrentModes) != 0 {
		t.Errorf("bob CurrentModes: got %d, want 0", len(bob.CurrentModes))
	}

	// Carol should have single mode
	carol, ok := got["~carol"]
	if !ok {
		t.Fatal("carol not found in loaded users")
	}
	assertHasMode(carol.PreservedModes, "#chan1", []string{"v"})
	assertHasMode(carol.CurrentModes, "#chan1", []string{"v"})
}
