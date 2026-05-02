package participant

import (
	"sync"
	"time"
)

// MessageContext holds context information for generating responses
type MessageContext struct {
	Type              string   // "reactive", etc.
	TimeOfDay         string   // "morning", "afternoon", "evening", "night"
	ChannelActivity   string   // "quiet", "active", "very_active"
	RecentMessages    []string // Last few messages for context
	UserRelation      string   // "stranger", "regular", "companion"
	PersonalityMode   string   // "friendly", "companion", "casual"
	DaysSinceLastChat int      // For companion check-ins
	IsHighlighted     bool     // Bot was mentioned/highlighted
	TriggerUser       string   // Username that triggered the response
}

// ConversationMemory stores recent channel activity and user relationships
type ConversationMemory struct {
	mu               sync.RWMutex
	NetworkName      string
	ChannelName      string
	RecentMessages   []ContextMessage
	UserProfiles     map[string]UserProfile
	LastInteractions map[string]time.Time
	LastBotMessage   time.Time
	MessageCount     int                    // Messages since last bot response
	BurstTimers      map[string]*time.Timer // Per-user burst response timers
	PendingResponses map[string]bool        // Users with pending burst responses
}

// ContextMessage represents a message in the conversation history
type ContextMessage struct {
	Username  string
	Message   string
	Timestamp time.Time
	IsBot     bool
}

// UserProfile stores information about individual users
type UserProfile struct {
	Username     string
	FirstSeen    time.Time
	LastSeen     time.Time
	Relationship string   // "stranger", "regular", "companion"
	Topics       []string // Topics they've discussed
	MessageCount int      // Total messages seen from this user
	IsActive     bool     // Recently active user
}

// PersonalityConfig defines personality traits and prompts
type PersonalityConfig struct {
	Name          string
	SystemPrompt  string
	Chattiness    float64 // 0.0-1.0, affects response probability
	CompanionMode bool    // Special behaviors for companion relationships
}
