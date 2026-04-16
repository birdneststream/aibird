package participant

import (
	"fmt"
	"strings"
	"time"

	"aibird/irc/channels"
	"aibird/irc/networks"
	"aibird/logger"
	"aibird/settings"

	"github.com/lrstanley/girc"
)

// Participant manages the AI participant system
type Participant struct {
	config   *settings.Config
	memory   map[string]*ConversationMemory // key: network:channel
	state    map[string]*ParticipantState   // key: network:channel
	schedule *Schedule
}

// NewParticipant creates a new participant system
func NewParticipant(config *settings.Config) *Participant {
	return &Participant{
		config:   config,
		memory:   make(map[string]*ConversationMemory),
		state:    make(map[string]*ParticipantState),
		schedule: loadSchedule(config),
	}
}

// HandleChatMessage processes non-command messages for potential responses
func (p *Participant) HandleChatMessage(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config) {
	// Quick check if chat mode is enabled for this channel
	channel := network.GetNetworkChannel(e.Params[0])
	if channel == nil || !channel.ChatMode {
		return
	}

	// Get or create memory for this channel
	memory := p.getOrCreateMemory(network.NetworkName, channel.Name)

	// Record this message in memory
	p.recordMessage(memory, e.Source.Name, e.Last(), false)

	// Check if we should respond
	if p.shouldRespond(memory, channel, network, e) {
		logger.Debug("Participant deciding to respond", "network", network.NetworkName, "channel", channel.Name, "trigger", e.Source.Name)

		// Check if this user is in a message burst - if so, delay response
		if p.isUserInBurst(memory, e.Source.Name) {
			logger.Debug("User in message burst, delaying response", "user", e.Source.Name)
			// Start/reset burst timer for this user
			go p.handleBurstResponse(c, e, network, channel, memory, 3*time.Second)
		} else {
			// Generate and send response immediately for single messages
			go p.generateAndSendResponse(c, e, network, channel, memory)
		}
	}
}

// shouldRespond determines if the bot should respond to a message
func (p *Participant) shouldRespond(memory *ConversationMemory, channel *channels.Channel, network *networks.Network, e girc.Event) bool {
	// Don't respond to ourselves
	if e.Source.Name == network.Nick {
		return false
	}

	// Check if bot is highlighted/mentioned (ALWAYS respond to highlights, ignore cooldown)
	isHighlighted := strings.Contains(strings.ToLower(e.Last()), strings.ToLower(network.Nick))

	if isHighlighted {
		logger.Debug("Bot highlighted, will respond (ignoring cooldown)", "message", e.Last())
		return true
	}

	// For non-highlighted messages, check cooldown
	if time.Since(memory.LastBotMessage) < 1*time.Second {
		logger.Debug("Skipping response due to cooldown", "time_since_last", time.Since(memory.LastBotMessage))
		return false
	}

	// Natural conversation participation based on personality and activity
	personality := p.getChannelPersonality(channel)

	// Higher chance to respond if:
	// 1. Channel has been quiet (encourage conversation)
	// 2. There's ongoing conversation (participate naturally)
	// 3. Based on chatResponseRate setting

	baseRate := channel.ChatResponseRate
	if baseRate <= 0 {
		baseRate = personality.Chattiness
	}

	// Increase response rate if channel has been quiet
	if p.analyzeChannelActivity(memory) == "quiet" && memory.MessageCount > 0 {
		baseRate *= 2.0 // Double rate for quiet channels
	}

	// Increase rate if there's been recent back-and-forth conversation
	recentMessages := p.getRecentNonBotMessages(memory, 3)
	if len(recentMessages) >= 2 {
		baseRate *= 1.5 // Increase rate during active conversations
	}

	// Check if message seems directed at bot (questions, mentions partial name, etc)
	message := strings.ToLower(e.Last())
	botNameParts := strings.ToLower(network.Nick)

	// Higher rate for messages that seem directed at the bot
	if strings.Contains(message, "?") || // Questions
		strings.Contains(message, "bot") || // Generic bot references
		(len(botNameParts) >= 3 && strings.Contains(message, botNameParts[:3])) || // Partial name match
		strings.HasSuffix(message, "?") { // Ends with question
		baseRate *= 2.0 // Double rate for potentially directed messages
		logger.Debug("Message seems directed at bot, increasing rate", "baseRate", baseRate)
	}

	// Random chance based on calculated rate
	if baseRate > 0 {
		// Use time-based pseudo-random for consistency
		seed := time.Now().UnixNano() % 100
		if float64(seed)/100.0 < baseRate {
			logger.Debug("Bot deciding to participate in conversation", "baseRate", baseRate, "message", e.Last())
			return true
		}
	}

	return false
}

// generateAndSendResponse creates and sends an AI-generated response
func (p *Participant) generateAndSendResponse(c *girc.Client, e girc.Event, network *networks.Network, channel *channels.Channel, memory *ConversationMemory) {
	// Build context for response generation
	ctx := p.buildMessageContext(memory, channel, network, e, "reactive")

	// Generate response using OpenRouter
	response, err := GenerateParticipantMessage(ctx, p.config.OpenRouter)
	if err != nil {
		logger.Error("Failed to generate participant response", "error", err, "network", network.NetworkName, "channel", channel.Name)
		return
	}

	if response == "" {
		logger.Debug("Empty response generated, skipping", "network", network.NetworkName, "channel", channel.Name)
		return
	}

	// Send the response
	logger.Info("Participant sending response", "network", network.NetworkName, "channel", channel.Name, "response", response)
	c.Cmd.Message(channel.Name, response)

	// Update memory with bot's response
	p.recordMessage(memory, network.Nick, response, true)
}

// buildMessageContext creates context for response generation
func (p *Participant) buildMessageContext(memory *ConversationMemory, channel *channels.Channel, network *networks.Network, e girc.Event, messageType string) MessageContext {
	personality := p.getChannelPersonality(channel)

	return MessageContext{
		Type:            messageType,
		TimeOfDay:       p.getTimeOfDay(),
		ChannelActivity: p.analyzeChannelActivity(memory),
		RecentMessages:  p.getRecentMessages(memory, 20),
		UserRelation:    p.getUserRelation(memory, e.Source.Name),
		PersonalityMode: personality.Name,
		IsHighlighted:   p.isHighlighted(e.Last(), network.Nick),
		TriggerUser:     e.Source.Name,
	}
}

// getOrCreateMemory gets or creates conversation memory for a channel
func (p *Participant) getOrCreateMemory(networkName, channelName string) *ConversationMemory {
	key := fmt.Sprintf("%s:%s", networkName, channelName)

	if memory, exists := p.memory[key]; exists {
		return memory
	}

	// Create new memory
	memory := &ConversationMemory{
		NetworkName:      networkName,
		ChannelName:      channelName,
		RecentMessages:   make([]ContextMessage, 0),
		UserProfiles:     make(map[string]UserProfile),
		LastInteractions: make(map[string]time.Time),
		BurstTimers:      make(map[string]*time.Timer),
		PendingResponses: make(map[string]bool),
	}

	p.memory[key] = memory
	return memory
}

// recordMessage adds a message to conversation memory
func (p *Participant) recordMessage(memory *ConversationMemory, username, message string, isBot bool) {
	now := time.Now()

	// Add to recent messages (keep last 50 for much better context)
	contextMsg := ContextMessage{
		Username:  username,
		Message:   message,
		Timestamp: now,
		IsBot:     isBot,
	}

	memory.RecentMessages = append(memory.RecentMessages, contextMsg)
	if len(memory.RecentMessages) > 50 {
		memory.RecentMessages = memory.RecentMessages[1:]
	}

	// Update user profile
	if !isBot {
		profile := memory.UserProfiles[username]
		profile.Username = username
		profile.LastSeen = now
		profile.MessageCount++
		profile.IsActive = true

		if profile.FirstSeen.IsZero() {
			profile.FirstSeen = now
			profile.Relationship = "stranger"
		}

		memory.UserProfiles[username] = profile
		memory.LastInteractions[username] = now
	} else {
		memory.LastBotMessage = now
	}

	memory.MessageCount++
}

// Helper functions

func (p *Participant) getChannelPersonality(channel *channels.Channel) PersonalityConfig {
	// Default personality
	return PersonalityConfig{
		Name:          "friendly",
		SystemPrompt:  "You are a friendly, casual IRC user. Keep responses short and conversational.",
		Chattiness:    0.3,
		CompanionMode: false,
	}
}

func (p *Participant) getTimeOfDay() string {
	hour := time.Now().Hour()
	switch {
	case hour >= 5 && hour < 12:
		return "morning"
	case hour >= 12 && hour < 17:
		return "afternoon"
	case hour >= 17 && hour < 21:
		return "evening"
	default:
		return "night"
	}
}

func (p *Participant) analyzeChannelActivity(memory *ConversationMemory) string {
	// Simple analysis based on recent message count
	recentCount := 0
	cutoff := time.Now().Add(-10 * time.Minute)

	for _, msg := range memory.RecentMessages {
		if msg.Timestamp.After(cutoff) && !msg.IsBot {
			recentCount++
		}
	}

	switch {
	case recentCount >= 5:
		return "very_active"
	case recentCount >= 2:
		return "active"
	default:
		return "quiet"
	}
}

func (p *Participant) getRecentMessages(memory *ConversationMemory, limit int) []string {
	var messages []string
	start := len(memory.RecentMessages) - limit
	if start < 0 {
		start = 0
	}

	for i := start; i < len(memory.RecentMessages); i++ {
		msg := memory.RecentMessages[i]
		if !msg.IsBot { // Don't include bot's own messages in context
			messages = append(messages, fmt.Sprintf("%s: %s", msg.Username, msg.Message))
		}
	}

	return messages
}

func (p *Participant) getRecentNonBotMessages(memory *ConversationMemory, limit int) []string {
	var messages []string
	count := 0

	// Go backwards through recent messages to get the most recent non-bot messages
	for i := len(memory.RecentMessages) - 1; i >= 0 && count < limit; i-- {
		msg := memory.RecentMessages[i]
		if !msg.IsBot {
			messages = append([]string{fmt.Sprintf("%s: %s", msg.Username, msg.Message)}, messages...)
			count++
		}
	}

	return messages
}

func (p *Participant) getUserRelation(memory *ConversationMemory, username string) string {
	profile, exists := memory.UserProfiles[username]
	if !exists {
		return "stranger"
	}

	return profile.Relationship
}

func (p *Participant) isHighlighted(message, botNick string) bool {
	return strings.Contains(strings.ToLower(message), strings.ToLower(botNick))
}

// loadSchedule creates a default schedule - will be enhanced later
func loadSchedule(config *settings.Config) *Schedule {
	return &Schedule{
		Timezone:    "America/New_York",
		ActiveHours: "08:00-23:00",
		WeeklySchedule: map[string]DaySchedule{
			"monday":    {Active: true, Hours: "08:00-23:00"},
			"tuesday":   {Active: true, Hours: "08:00-23:00"},
			"wednesday": {Active: true, Hours: "08:00-23:00"},
			"thursday":  {Active: true, Hours: "08:00-23:00"},
			"friday":    {Active: true, Hours: "08:00-23:00"},
			"saturday":  {Active: true, Hours: "09:00-24:00"},
			"sunday":    {Active: true, Hours: "09:00-23:00"},
		},
		SleepPattern: SleepPattern{
			TransitionMinutes:   30,
			WakeGreeting:        true,
			SleepGreeting:       true,
			GreetingWindow:      30,
			GreetingProbability: 0.7,
		},
	}
}

// Global participant instance - will be initialized in main.go
var GlobalParticipant *Participant

// InitParticipant initializes the global participant system
func InitParticipant(config *settings.Config) {
	GlobalParticipant = NewParticipant(config)
	logger.Info("Participant system initialized")
}

// isUserInBurst checks if a user is sending messages rapidly (within 5 seconds)
func (p *Participant) isUserInBurst(memory *ConversationMemory, username string) bool {
	// Look at recent messages from this user
	recentUserMessages := 0
	cutoff := time.Now().Add(-5 * time.Second) // 5 second window

	for _, msg := range memory.RecentMessages {
		if msg.Username == username && msg.Timestamp.After(cutoff) && !msg.IsBot {
			recentUserMessages++
		}
	}

	// Consider it a burst if user sent 2+ messages in 5 seconds
	return recentUserMessages >= 2
}

// handleBurstResponse manages delayed responses for users sending message bursts
func (p *Participant) handleBurstResponse(c *girc.Client, e girc.Event, network *networks.Network, channel *channels.Channel, memory *ConversationMemory, delay time.Duration) {
	username := e.Source.Name

	// Cancel any existing timer for this user
	if timer, exists := memory.BurstTimers[username]; exists {
		timer.Stop()
	}

	// Mark this user as having a pending response
	memory.PendingResponses[username] = true

	// Create new timer
	memory.BurstTimers[username] = time.AfterFunc(delay, func() {
		// Check if response is still pending (not cancelled by another message)
		if memory.PendingResponses[username] {
			logger.Debug("Burst delay expired, sending response", "user", username)

			// Generate response with full context including all burst messages
			ctx := p.buildMessageContext(memory, channel, network, e, "reactive")
			response, err := GenerateParticipantMessage(ctx, p.config.OpenRouter)
			if err != nil {
				logger.Error("Failed to generate burst response", "error", err)
				return
			}

			if response != "" {
				logger.Info("Participant sending burst response", "network", network.NetworkName, "channel", channel.Name, "response", response)
				c.Cmd.Message(channel.Name, response)
				p.recordMessage(memory, network.Nick, response, true)
			}

			// Clean up
			delete(memory.PendingResponses, username)
			delete(memory.BurstTimers, username)
		}
	})
}

// HandleChatMessage is the main entry point for processing chat messages
func HandleChatMessage(c *girc.Client, e girc.Event, network *networks.Network, config *settings.Config) {
	if GlobalParticipant != nil {
		GlobalParticipant.HandleChatMessage(c, e, network, config)
	}
}
