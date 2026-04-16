package participant

import (
	"time"
)

// MessageContext holds context information for generating responses
type MessageContext struct {
	Type              string   // "wake_greeting", "sleep_message", "spontaneous", "check_in", "activity_response", "reactive"
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

// Schedule represents the bot's activity schedule
type Schedule struct {
	Timezone       string
	ActiveHours    string // "08:00-23:00"
	WeeklySchedule map[string]DaySchedule
	SleepPattern   SleepPattern
}

// DaySchedule represents schedule for a specific day
type DaySchedule struct {
	Active bool
	Hours  string
}

// SleepPattern controls sleep/wake behavior
type SleepPattern struct {
	TransitionMinutes   int     // Minutes for gradual sleep/wake
	WakeGreeting        bool    // Send morning greetings
	SleepGreeting       bool    // Send goodnight messages
	GreetingWindow      int     // Minutes after wake to potentially greet
	GreetingProbability float64 // Chance to send greeting
}

// ActivityTrigger defines conditions for activity-based responses
type ActivityTrigger struct {
	Type        string  // "join", "part", "quiet_channel", "topic_change"
	MinDelay    int     // Minimum minutes before responding
	MaxDelay    int     // Maximum delay variation
	Probability float64 // Chance to respond to this trigger
	Enabled     bool
}

// SpontaneousConfig controls proactive messaging
type SpontaneousConfig struct {
	Enabled        bool
	MinInterval    int     // Minimum minutes between spontaneous messages
	MaxInterval    int     // Maximum minutes
	Probability    float64 // Base chance per interval
	ActivityBased  bool    // Wait for some activity before sending
	MaxDailyCount  int     // Maximum spontaneous messages per day
	QuietThreshold int     // Minutes of silence before considering channel "quiet"
}

// ParticipantState tracks the current state of the participant system
type ParticipantState struct {
	NetworkName            string
	ChannelName            string
	IsActive               bool // Currently active based on schedule
	LastSpontaneousMessage time.Time
	LastWakeGreeting       time.Time
	LastSleepGreeting      time.Time
	DailySpontaneousCount  int
	SleepState             string // "awake", "drowsy", "asleep", "waking"
}

// PersonalityConfig defines personality traits and prompts
type PersonalityConfig struct {
	Name          string
	SystemPrompt  string
	Chattiness    float64 // 0.0-1.0, affects response probability
	CompanionMode bool    // Special behaviors for companion relationships
}

// ParticipantConfig holds all configuration for the participant system
type ParticipantConfig struct {
	Enabled          bool
	Model            string
	MaxTokens        int
	Temperature      float64
	ContextMessages  int
	Personalities    map[string]PersonalityConfig
	Schedule         Schedule
	Spontaneous      SpontaneousConfig
	ActivityTriggers map[string]ActivityTrigger
}
