package birdbase

import "time"

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type LeaderboardEntry struct {
	Nickname  string    `json:"nickname"`
	Command   string    `json:"command"`
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updated_at"`
}

type GlobalLeaderboardEntry struct {
	Network   string    `json:"network"`
	Nickname  string    `json:"nickname"`
	Command   string    `json:"command"`
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updated_at"`
}

type UserTotalEntry struct {
	Nickname   string    `json:"nickname"`
	TotalCount int       `json:"total_count"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type NetworkData struct {
	Enabled       bool
	Nick          string
	User          string
	Name          string
	Pass          string
	PreserveModes bool
	NickServPass  string
	PingDelay     int
	Version       string
	Throttle      int
	Burst         int
	ActionTrigger string
	ModesAtOnce   int
	IgnoredNicks  []string
	DenyCommands  []string
	AdminHosts    []AdminHost
	Servers       []ServerData
	Channels      []ChannelData
}

type AdminHost struct {
	Host  string
	Ident string
	Owner bool
}

type ServerData struct {
	Host          string
	Port          int
	SSL           bool
	SkipSSLVerify bool
}

type ChannelData struct {
	Name          string
	PreserveModes bool
	Ai            bool
	Sd            bool
	ImageDescribe bool
	Sound         bool
	Video         bool
	ActionTrigger string
	TrimOutput    bool
	DenyCommands  []string
}

type UserData struct {
	ID             int
	NetworkID      int
	NickName       string
	Ident          string
	Host           string
	FirstSeen      int64
	LatestActivity int64
	LatestChat     string
	IsAdmin        bool
	IsOwner        bool
	Ignored        bool
	AccessLevel    int
	AiService      string
	AiModel        string
	AiBasePrompt   string
	AiPersonality  string
	PreservedModes []UserModeData
	CurrentModes   []UserModeData
}

type UserModeData struct {
	Channel string
	Modes   []string
}
