package participant

import (
	"strings"
	"testing"
	"time"

	"aibird/irc/channels"
	"aibird/irc/networks"
	"aibird/logger"
	"aibird/settings"
	"aibird/text"

	"github.com/lrstanley/girc"
	"github.com/stretchr/testify/assert"
)

func init() {
	// Initialize logger to prevent nil-pointer panics in participant methods
	logger.Init(logger.Config{Level: "error", Format: "text"})
}

// newTestParticipant creates a Participant with nil-safe config for testing.
func newTestParticipant() *Participant {
	return NewParticipant(&settings.Config{})
}

// newTestMemory creates a fresh ConversationMemory for testing.
func newTestMemory() *ConversationMemory {
	return &ConversationMemory{
		NetworkName:      "testNet",
		ChannelName:      "#test",
		RecentMessages:   make([]ContextMessage, 0),
		UserProfiles:     make(map[string]UserProfile),
		LastInteractions: make(map[string]time.Time),
		BurstTimers:      make(map[string]*time.Timer),
		PendingResponses: make(map[string]bool),
	}
}

// addTestMessages is a helper that adds N non-bot messages to memory.
func addTestMessages(p *Participant, mem *ConversationMemory, username string, count int) {
	for i := 0; i < count; i++ {
		p.recordMessage(mem, username, "test message", false)
	}
}

// newTestEvent creates a girc.Event suitable for participant tests.
func newTestEvent(channel, sourceName, message string) girc.Event {
	return girc.Event{
		Command: "PRIVMSG",
		Params:  []string{channel, message},
		Source:  &girc.Source{Name: sourceName, Ident: "user", Host: "host"},
	}
}

// =============================================================================
// cleanParticipantResponse tests
// =============================================================================

func TestCleanParticipantResponse_TrimSpace(t *testing.T) {
	assert.Equal(t, "hello", cleanParticipantResponse("  hello  "))
}

func TestCleanParticipantResponse_RemovesAsterisks(t *testing.T) {
	assert.Equal(t, "waves hello", cleanParticipantResponse("*waves hello*"))
}

func TestCleanParticipantResponse_RemovesAIPhrases(t *testing.T) {
	phrases := []string{
		"As an AI",
		"I'm an AI",
		"As a helpful assistant",
		"I'd be happy to",
		"I hope this helps",
	}
	for _, phrase := range phrases {
		t.Run(phrase, func(t *testing.T) {
			input := phrase + " something else"
			result := cleanParticipantResponse(input)
			assert.NotContains(t, result, phrase, "AI phrase should be removed")
		})
	}
}

func TestCleanParticipantResponse_RemovesWrappingQuotes(t *testing.T) {
	assert.Equal(t, "hello world", cleanParticipantResponse("\"hello world\""))
}

func TestCleanParticipantResponse_PreservesInternalQuotes(t *testing.T) {
	result := cleanParticipantResponse("he said \"hello\" to me")
	assert.Contains(t, result, "\"hello\"")
}

func TestCleanParticipantResponse_TruncatesLongResponses(t *testing.T) {
	longMsg := strings.Repeat("This is a sentence. ", 25)
	result := cleanParticipantResponse(longMsg)
	assert.LessOrEqual(t, len(result), 400, "long response should be truncated")
}

func TestCleanParticipantResponse_TruncatesToLastCompleteSentence(t *testing.T) {
	sentences := make([]string, 30)
	for i := range sentences {
		sentences[i] = "Short sentence"
	}
	longMsg := strings.Join(sentences, ". ")
	result := cleanParticipantResponse(longMsg)
	if result != "" {
		assert.True(t, strings.HasSuffix(result, "."), "truncated response should end with complete sentence, got: %q", result)
	}
}

func TestCleanParticipantResponse_ShortResponseUnchanged(t *testing.T) {
	input := "Hello there, how are you?"
	assert.Equal(t, input, cleanParticipantResponse(input))
}

func TestCleanParticipantResponse_EmptyInput(t *testing.T) {
	assert.Equal(t, "", cleanParticipantResponse(""))
}

func TestCleanParticipantResponse_OnlyAsterisks(t *testing.T) {
	assert.Equal(t, "", cleanParticipantResponse("***"))
}

// =============================================================================
// getPersonalityPrompt tests
// =============================================================================

func TestGetPersonalityPrompt_Companion(t *testing.T) {
	prompt := getPersonalityPrompt("companion")
	assert.Contains(t, prompt, "companion")
	assert.Contains(t, prompt, "affectionate")
}

func TestGetPersonalityPrompt_DefaultFriendly(t *testing.T) {
	prompt := getPersonalityPrompt("friendly")
	assert.Contains(t, prompt, "friendly")
	assert.Contains(t, prompt, "casual")
}

func TestGetPersonalityPrompt_Unknown(t *testing.T) {
	prompt := getPersonalityPrompt("nonexistent")
	assert.Contains(t, prompt, "friendly")
}

func TestGetPersonalityPrompt_Empty(t *testing.T) {
	prompt := getPersonalityPrompt("")
	assert.Contains(t, prompt, "friendly")
}

// =============================================================================
// buildContextualPrompt tests
// =============================================================================

func TestBuildContextualPrompt_Reactive(t *testing.T) {
	ctx := &MessageContext{
		Type:            "reactive",
		TimeOfDay:       "morning",
		ChannelActivity: "active",
		PersonalityMode: "friendly",
		TriggerUser:     "alice",
	}
	prompt := buildContextualPrompt(ctx)
	assert.Contains(t, prompt, "morning")
	assert.Contains(t, prompt, "Respond naturally")
}

func TestBuildContextualPrompt_ReactiveHighlighted(t *testing.T) {
	ctx := &MessageContext{
		Type:            "reactive",
		TimeOfDay:       "evening",
		ChannelActivity: "quiet",
		PersonalityMode: "friendly",
		IsHighlighted:   true,
		TriggerUser:     "bob",
	}
	prompt := buildContextualPrompt(ctx)
	assert.Contains(t, prompt, "mentioned by bob")
}

func TestBuildContextualPrompt_ReactiveNotHighlighted(t *testing.T) {
	ctx := &MessageContext{
		Type:            "reactive",
		TimeOfDay:       "morning",
		ChannelActivity: "quiet",
		PersonalityMode: "friendly",
		IsHighlighted:   false,
	}
	prompt := buildContextualPrompt(ctx)
	assert.NotContains(t, prompt, "mentioned by")
}

func TestBuildContextualPrompt_WakeGreeting(t *testing.T) {
	ctx := &MessageContext{
		Type:            "wake_greeting",
		TimeOfDay:       "morning",
		ChannelActivity: "quiet",
		PersonalityMode: "friendly",
	}
	prompt := buildContextualPrompt(ctx)
	assert.Contains(t, prompt, "woke up")
	assert.Contains(t, prompt, "morning")
}

func TestBuildContextualPrompt_SleepMessage(t *testing.T) {
	ctx := &MessageContext{
		Type:            "sleep_message",
		TimeOfDay:       "night",
		ChannelActivity: "quiet",
		PersonalityMode: "friendly",
	}
	prompt := buildContextualPrompt(ctx)
	assert.Contains(t, prompt, "sleepy")
	assert.Contains(t, prompt, "goodnight")
}

func TestBuildContextualPrompt_Spontaneous(t *testing.T) {
	ctx := &MessageContext{
		Type:            "spontaneous",
		TimeOfDay:       "afternoon",
		PersonalityMode: "friendly",
		RecentMessages:  []string{"alice: hey", "bob: sup"},
	}
	prompt := buildContextualPrompt(ctx)
	assert.Contains(t, prompt, "conversation starter")
	assert.Contains(t, prompt, "alice: hey")
}

func TestBuildContextualPrompt_SpontaneousEmptyRecent(t *testing.T) {
	ctx := &MessageContext{
		Type:            "spontaneous",
		TimeOfDay:       "afternoon",
		PersonalityMode: "friendly",
		RecentMessages:  nil,
	}
	prompt := buildContextualPrompt(ctx)
	assert.Contains(t, prompt, "conversation starter")
	assert.NotContains(t, prompt, "Recent messages:")
}

func TestBuildContextualPrompt_CheckIn(t *testing.T) {
	ctx := &MessageContext{
		Type:              "check_in",
		TimeOfDay:         "afternoon",
		PersonalityMode:   "friendly",
		TriggerUser:       "alice",
		DaysSinceLastChat: 5,
	}
	prompt := buildContextualPrompt(ctx)
	assert.Contains(t, prompt, "alice")
	assert.Contains(t, prompt, "5 days")
}

func TestBuildContextualPrompt_ActivityResponse(t *testing.T) {
	ctx := &MessageContext{
		Type:            "activity_response",
		TimeOfDay:       "evening",
		ChannelActivity: "active",
		PersonalityMode: "friendly",
		TriggerUser:     "newuser",
	}
	prompt := buildContextualPrompt(ctx)
	assert.Contains(t, prompt, "newuser")
	assert.Contains(t, prompt, "welcome")
}

func TestBuildContextualPrompt_UnknownType(t *testing.T) {
	ctx := &MessageContext{Type: "nonexistent"}
	prompt := buildContextualPrompt(ctx)
	assert.Empty(t, prompt, "unknown type should return empty prompt")
}

// =============================================================================
// GenerateParticipantMessage guard tests
// =============================================================================

func TestGenerateParticipantMessage_NoAPIKey(t *testing.T) {
	ctx := &MessageContext{Type: "reactive", PersonalityMode: "friendly"}
	_, err := GenerateParticipantMessage(ctx, &settings.GlmConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestGenerateParticipantMessage_NoURL(t *testing.T) {
	ctx := &MessageContext{Type: "reactive", PersonalityMode: "friendly"}
	_, err := GenerateParticipantMessage(ctx, &settings.GlmConfig{ApiKey: "test-key"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestGenerateParticipantMessage_EmptyPromptType(t *testing.T) {
	ctx := &MessageContext{Type: "unknown_type", PersonalityMode: "friendly"}
	_, err := GenerateParticipantMessage(ctx, &settings.GlmConfig{ApiKey: "key", Url: "http://localhost"})
	// Should fail because buildContextualPrompt returns empty for unknown type
	assert.Error(t, err)
}

// =============================================================================
// isHighlighted tests
// =============================================================================

func TestIsHighlighted_ExactMatch(t *testing.T) {
	p := newTestParticipant()
	assert.True(t, p.isHighlighted("Hey BirdBot how are you?", "BirdBot"))
}

func TestIsHighlighted_CaseInsensitive(t *testing.T) {
	p := newTestParticipant()
	assert.True(t, p.isHighlighted("hey birdbot!", "BirdBot"))
	assert.True(t, p.isHighlighted("BIRDBOT hello", "birdbot"))
}

func TestIsHighlighted_NotPresent(t *testing.T) {
	p := newTestParticipant()
	assert.False(t, p.isHighlighted("hello world", "BirdBot"))
}

func TestIsHighlighted_EmptyMessage(t *testing.T) {
	p := newTestParticipant()
	assert.False(t, p.isHighlighted("", "BirdBot"))
}

// =============================================================================
// recordMessage tests
// =============================================================================

func TestRecordMessage_AddsToRecent(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)

	mem.mu.RLock()
	defer mem.mu.RUnlock()
	assert.Len(t, mem.RecentMessages, 1)
	assert.Equal(t, "alice", mem.RecentMessages[0].Username)
	assert.Equal(t, "hello", mem.RecentMessages[0].Message)
	assert.False(t, mem.RecentMessages[0].IsBot)
}

func TestRecordMessage_UpdatesUserProfile(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)

	mem.mu.RLock()
	profile := mem.UserProfiles["alice"]
	mem.mu.RUnlock()
	assert.Equal(t, "alice", profile.Username)
	assert.Equal(t, 1, profile.MessageCount)
	assert.True(t, profile.IsActive)
	assert.Equal(t, "stranger", profile.Relationship)
	assert.False(t, profile.FirstSeen.IsZero())
}

func TestRecordMessage_UpdatesLastSeen(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)
	p.recordMessage(mem, "alice", "world", false)

	mem.mu.RLock()
	profile := mem.UserProfiles["alice"]
	mem.mu.RUnlock()
	assert.Equal(t, 2, profile.MessageCount)
	// LastSeen should be updated; FirstSeen should remain the earliest
	assert.False(t, profile.LastSeen.Before(profile.FirstSeen), "LastSeen should not be before FirstSeen")
}

func TestRecordMessage_BotMessage(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "BirdBot", "I am a bot", true)

	mem.mu.RLock()
	defer mem.mu.RUnlock()
	assert.Len(t, mem.RecentMessages, 1)
	assert.True(t, mem.RecentMessages[0].IsBot)
	_, exists := mem.UserProfiles["BirdBot"]
	assert.False(t, exists)
	assert.False(t, mem.LastBotMessage.IsZero())
}

func TestRecordMessage_IncrementsMessageCount(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "msg1", false)
	p.recordMessage(mem, "bob", "msg2", false)
	p.recordMessage(mem, "BirdBot", "msg3", true)

	mem.mu.RLock()
	defer mem.mu.RUnlock()
	assert.Equal(t, 3, mem.MessageCount)
}

func TestRecordMessage_CapsAt50(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	for i := 0; i < 55; i++ {
		p.recordMessage(mem, "alice", "msg", false)
	}

	mem.mu.RLock()
	defer mem.mu.RUnlock()
	assert.Len(t, mem.RecentMessages, 50, "should cap at 50 messages")
}

func TestRecordMessage_FirstSeenOnlyOnce(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "first", false)

	mem.mu.RLock()
	firstSeen := mem.UserProfiles["alice"].FirstSeen
	mem.mu.RUnlock()

	p.recordMessage(mem, "alice", "second", false)
	mem.mu.RLock()
	profile := mem.UserProfiles["alice"]
	mem.mu.RUnlock()

	assert.Equal(t, firstSeen, profile.FirstSeen, "FirstSeen should not change")
	assert.Equal(t, 2, profile.MessageCount)
}

func TestRecordMessage_UpdatesLastInteraction(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)

	mem.mu.RLock()
	lastInteraction := mem.LastInteractions["alice"]
	mem.mu.RUnlock()

	assert.False(t, lastInteraction.IsZero(), "LastInteractions should be updated for non-bot messages")
}

func TestRecordMessage_BotDoesNotUpdateLastInteraction(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "BirdBot", "response", true)

	mem.mu.RLock()
	_, exists := mem.LastInteractions["BirdBot"]
	mem.mu.RUnlock()

	assert.False(t, exists, "LastInteractions should not be updated for bot messages")
}

// =============================================================================
// getOrCreateMemory tests
// =============================================================================

func TestGetOrCreateMemory_CreatesNew(t *testing.T) {
	p := newTestParticipant()
	mem := p.getOrCreateMemory("testNet", "#test")

	assert.NotNil(t, mem)
	assert.Equal(t, "testNet", mem.NetworkName)
	assert.Equal(t, "#test", mem.ChannelName)
	assert.NotNil(t, mem.RecentMessages)
	assert.NotNil(t, mem.UserProfiles)
}

func TestGetOrCreateMemory_ReturnsSame(t *testing.T) {
	p := newTestParticipant()
	mem1 := p.getOrCreateMemory("testNet", "#test")
	mem2 := p.getOrCreateMemory("testNet", "#test")

	assert.Same(t, mem1, mem2, "should return same memory instance")
}

func TestGetOrCreateMemory_DifferentChannels(t *testing.T) {
	p := newTestParticipant()
	mem1 := p.getOrCreateMemory("testNet", "#chan1")
	mem2 := p.getOrCreateMemory("testNet", "#chan2")

	assert.NotSame(t, mem1, mem2)
}

func TestGetOrCreateMemory_DifferentNetworks(t *testing.T) {
	p := newTestParticipant()
	mem1 := p.getOrCreateMemory("net1", "#test")
	mem2 := p.getOrCreateMemory("net2", "#test")

	assert.NotSame(t, mem1, mem2)
}

func TestGetOrCreateMemory_Concurrent(t *testing.T) {
	p := newTestParticipant()
	done := make(chan *ConversationMemory, 10)

	// Concurrently create memory for the same key
	for i := 0; i < 10; i++ {
		go func() {
			done <- p.getOrCreateMemory("testNet", "#concurrent")
		}()
	}

	first := <-done
	for i := 0; i < 9; i++ {
		mem := <-done
		assert.Same(t, first, mem, "concurrent access should return same instance")
	}
}

// =============================================================================
// analyzeChannelActivity tests
// =============================================================================

func TestAnalyzeChannelActivity_Quiet(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	assert.Equal(t, "quiet", p.analyzeChannelActivity(mem))
}

func TestAnalyzeChannelActivity_Active(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	addTestMessages(p, mem, "alice", 2)
	assert.Equal(t, "active", p.analyzeChannelActivity(mem))
}

func TestAnalyzeChannelActivity_VeryActive(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	addTestMessages(p, mem, "alice", 5)
	assert.Equal(t, "very_active", p.analyzeChannelActivity(mem))
}

func TestAnalyzeChannelActivity_IgnoresBotMessages(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "Bot", "hi", true)
	p.recordMessage(mem, "Bot", "hello", true)
	assert.Equal(t, "quiet", p.analyzeChannelActivity(mem))
}

func TestAnalyzeChannelActivity_IgnoresOldMessages(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	mem.mu.Lock()
	cutoff := time.Now().Add(-15 * time.Minute)
	mem.RecentMessages = append(mem.RecentMessages,
		ContextMessage{Username: "alice", Message: "old", Timestamp: cutoff, IsBot: false},
		ContextMessage{Username: "alice", Message: "old2", Timestamp: cutoff, IsBot: false},
	)
	mem.mu.Unlock()

	assert.Equal(t, "quiet", p.analyzeChannelActivity(mem))
}

func TestAnalyzeChannelActivity_BoundaryActive(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	// Exactly 2 messages = active (boundary)
	addTestMessages(p, mem, "alice", 2)
	assert.Equal(t, "active", p.analyzeChannelActivity(mem))
}

func TestAnalyzeChannelActivity_BoundaryVeryActive(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	// Exactly 5 messages = very_active (boundary)
	addTestMessages(p, mem, "alice", 5)
	assert.Equal(t, "very_active", p.analyzeChannelActivity(mem))
}

// =============================================================================
// getUserRelation tests
// =============================================================================

func TestGetUserRelation_Stranger(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	assert.Equal(t, "stranger", p.getUserRelation(mem, "unknown"))
}

func TestGetUserRelation_FromProfile(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)
	assert.Equal(t, "stranger", p.getUserRelation(mem, "alice"))
}

// =============================================================================
// getRecentMessages tests
// =============================================================================

func TestGetRecentMessages_Empty(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	result := p.getRecentMessages(mem, 10)
	assert.Empty(t, result)
}

func TestGetRecentMessages_ExcludesBot(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)
	p.recordMessage(mem, "Bot", "hi there", true)

	result := p.getRecentMessages(mem, 10)
	assert.Len(t, result, 1)
	assert.Contains(t, result[0], "alice: hello")
}

func TestGetRecentMessages_RespectsLimit(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	for i := 0; i < 10; i++ {
		p.recordMessage(mem, "alice", "msg", false)
	}

	result := p.getRecentMessages(mem, 3)
	assert.Len(t, result, 3)
}

func TestGetRecentMessages_OnlyBotMessages(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "Bot", "hello", true)
	p.recordMessage(mem, "Bot", "world", true)

	result := p.getRecentMessages(mem, 10)
	assert.Empty(t, result, "should exclude all bot messages")
}

// =============================================================================
// getRecentNonBotMessages tests
// =============================================================================

func TestGetRecentNonBotMessages_Empty(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	result := p.getRecentNonBotMessages(mem, 5)
	assert.Empty(t, result)
}

func TestGetRecentNonBotMessages_ExcludesBot(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)
	p.recordMessage(mem, "Bot", "response", true)
	p.recordMessage(mem, "bob", "world", false)

	result := p.getRecentNonBotMessages(mem, 5)
	assert.Len(t, result, 2)
}

func TestGetRecentNonBotMessages_RespectsLimit(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	for i := 0; i < 10; i++ {
		p.recordMessage(mem, "alice", "msg", false)
	}

	result := p.getRecentNonBotMessages(mem, 3)
	assert.Len(t, result, 3)
}

func TestGetRecentNonBotMessages_OrderIsChronological(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "first", false)
	p.recordMessage(mem, "bob", "second", false)

	result := p.getRecentNonBotMessages(mem, 5)
	assert.Len(t, result, 2)
	assert.Contains(t, result[0], "alice: first")
	assert.Contains(t, result[1], "bob: second")
}

// =============================================================================
// isUserInBurst tests
// =============================================================================

func TestIsUserInBurst_SingleMessage(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)
	assert.False(t, p.isUserInBurst(mem, "alice"))
}

func TestIsUserInBurst_TwoRapidMessages(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)
	p.recordMessage(mem, "alice", "world", false)
	assert.True(t, p.isUserInBurst(mem, "alice"))
}

func TestIsUserInBurst_IgnoresOtherUsers(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)
	p.recordMessage(mem, "bob", "world", false)
	assert.False(t, p.isUserInBurst(mem, "alice"))
}

func TestIsUserInBurst_IgnoresBotMessages(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	p.recordMessage(mem, "alice", "hello", false)
	p.recordMessage(mem, "alice", "response", true)
	assert.False(t, p.isUserInBurst(mem, "alice"))
}

func TestIsUserInBurst_UnknownUser(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	assert.False(t, p.isUserInBurst(mem, "nobody"))
}

// =============================================================================
// getTimeOfDay tests
// =============================================================================

func TestGetTimeOfDay_ReturnsValidCategory(t *testing.T) {
	p := newTestParticipant()
	result := p.getTimeOfDay()
	valid := map[string]bool{"morning": true, "afternoon": true, "evening": true, "night": true}
	assert.True(t, valid[result], "expected valid time-of-day category, got: %s", result)
}

// =============================================================================
// getChannelPersonality tests
// =============================================================================

func TestGetChannelPersonality_Defaults(t *testing.T) {
	p := newTestParticipant()
	personality := p.getChannelPersonality(&channels.Channel{})
	assert.Equal(t, "friendly", personality.Name)
	assert.Equal(t, 0.3, personality.Chattiness)
	assert.False(t, personality.CompanionMode)
}

// =============================================================================
// HandleChatMessage early-return tests
// =============================================================================

func TestHandleChatMessage_NilChannel(t *testing.T) {
	p := newTestParticipant()
	network := &networks.Network{Nick: "BirdBot"}

	// Channel "nonexistent" not in network → GetNetworkChannel returns nil
	assert.NotPanics(t, func() {
		p.HandleChatMessage(nil, newTestEvent("#nonexistent", "alice", "hello"), network, &settings.Config{})
	})
}

func TestHandleChatMessage_ChatModeDisabled(t *testing.T) {
	p := newTestParticipant()
	network := &networks.Network{
		Nick: "BirdBot",
		Channels: []channels.Channel{
			{Name: "#test", ChatMode: false},
		},
	}

	// ChatMode=false → early return, nothing happens
	assert.NotPanics(t, func() {
		p.HandleChatMessage(nil, newTestEvent("#test", "alice", "hello"), network, &settings.Config{})
	})
}

// =============================================================================
// shouldRespond tests
// =============================================================================

func TestShouldRespond_IgnoresSelfMessages(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	channel := &channels.Channel{ChatMode: true}
	network := &networks.Network{Nick: "BirdBot"}
	event := newTestEvent("#test", "BirdBot", "hello")

	assert.False(t, p.shouldRespond(mem, channel, network, event))
}

func TestShouldRespond_HighlightedAlwaysResponds(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	channel := &channels.Channel{ChatMode: true}
	network := &networks.Network{Nick: "BirdBot"}
	event := newTestEvent("#test", "alice", "hey BirdBot what's up?")

	assert.True(t, p.shouldRespond(mem, channel, network, event))
}

func TestShouldRespond_CooldownPreventsResponse(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	channel := &channels.Channel{ChatMode: true}
	network := &networks.Network{Nick: "BirdBot"}

	mem.mu.Lock()
	mem.LastBotMessage = time.Now()
	mem.mu.Unlock()

	event := newTestEvent("#test", "alice", "hello there")
	assert.False(t, p.shouldRespond(mem, channel, network, event))
}

func TestShouldRespond_QuestionIncreasesRate(t *testing.T) {
	// Smoke test: shouldRespond uses crypto/rand so the result is non-deterministic.
	// This test verifies no panic and correct early-return behavior.
	p := newTestParticipant()
	mem := newTestMemory()
	channel := &channels.Channel{ChatMode: true, ChatResponseRate: 0}
	network := &networks.Network{Nick: "BirdBot"}

	// With zero response rate, only the question multiplier matters
	// This test just verifies no panic and the function runs correctly
	event := newTestEvent("#test", "alice", "what do you think?")
	// Result is random, so we just verify it doesn't panic
	_ = p.shouldRespond(mem, channel, network, event)
}

func TestShouldRespond_BotNamePartialMatch(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	channel := &channels.Channel{ChatMode: true, ChatResponseRate: 0}
	network := &networks.Network{Nick: "BirdBot"}

	// Smoke test: verifies the bot-name partial-match path doesn't panic.
	// The result is non-deterministic due to crypto/rand in shouldRespond.
	event := newTestEvent("#test", "alice", "hey bir how are you")
	_ = p.shouldRespond(mem, channel, network, event)
}

// =============================================================================
// NewParticipant tests
// =============================================================================

func TestNewParticipant(t *testing.T) {
	cfg := &settings.Config{}
	p := NewParticipant(cfg)
	assert.NotNil(t, p)
	assert.Equal(t, cfg, p.config)
	assert.NotNil(t, p.memory)
}

// =============================================================================
// GlobalParticipant tests
// =============================================================================

func TestHandleChatMessage_NilGlobalParticipant(t *testing.T) {
	orig := GlobalParticipant
	GlobalParticipant = nil
	defer func() { GlobalParticipant = orig }()

	assert.NotPanics(t, func() {
		HandleChatMessage(nil, newTestEvent("#test", "alice", "hello"), nil, nil)
	})
}

func TestInitParticipant(t *testing.T) {
	orig := GlobalParticipant
	defer func() { GlobalParticipant = orig }()

	InitParticipant(&settings.Config{})
	assert.NotNil(t, GlobalParticipant)
}

// =============================================================================
// Type construction tests
// =============================================================================

func TestParticipantRequest_Fields(t *testing.T) {
	req := ParticipantRequest{
		Model:       "test-model",
		Messages:    []text.Message{{Role: "user", Content: "hi"}},
		Stream:      false,
		MaxTokens:   100,
		Temperature: 0.7,
	}
	assert.Equal(t, "test-model", req.Model)
	assert.Len(t, req.Messages, 1)
	assert.False(t, req.Stream)
	assert.Equal(t, 100, req.MaxTokens)
	assert.Equal(t, 0.7, req.Temperature)
}

func TestParticipantResponse_EmptyChoices(t *testing.T) {
	resp := ParticipantResponse{}
	assert.Empty(t, resp.Choices)
}

func TestMessageContext_AllFields(t *testing.T) {
	ctx := MessageContext{
		Type:              "reactive",
		TimeOfDay:         "morning",
		ChannelActivity:   "active",
		RecentMessages:    []string{"hello"},
		UserRelation:      "regular",
		PersonalityMode:   "friendly",
		DaysSinceLastChat: 3,
		IsHighlighted:     true,
		TriggerUser:       "alice",
	}
	assert.Equal(t, "reactive", ctx.Type)
	assert.Equal(t, "morning", ctx.TimeOfDay)
	assert.Equal(t, "active", ctx.ChannelActivity)
	assert.Equal(t, []string{"hello"}, ctx.RecentMessages)
	assert.Equal(t, "regular", ctx.UserRelation)
	assert.Equal(t, "friendly", ctx.PersonalityMode)
	assert.Equal(t, 3, ctx.DaysSinceLastChat)
	assert.True(t, ctx.IsHighlighted)
	assert.Equal(t, "alice", ctx.TriggerUser)
}

func TestContextMessage_Fields(t *testing.T) {
	now := time.Now()
	msg := ContextMessage{
		Username:  "alice",
		Message:   "hello",
		Timestamp: now,
		IsBot:     false,
	}
	assert.Equal(t, "alice", msg.Username)
	assert.Equal(t, "hello", msg.Message)
	assert.Equal(t, now, msg.Timestamp)
	assert.False(t, msg.IsBot)
}

func TestUserProfile_Defaults(t *testing.T) {
	profile := UserProfile{Username: "alice"}
	assert.Equal(t, "alice", profile.Username)
	assert.Empty(t, profile.Topics)
	assert.Equal(t, 0, profile.MessageCount)
	assert.False(t, profile.IsActive)
}

func TestPersonalityConfig_Defaults(t *testing.T) {
	cfg := PersonalityConfig{Name: "test"}
	assert.Equal(t, "test", cfg.Name)
	assert.Equal(t, 0.0, cfg.Chattiness)
	assert.False(t, cfg.CompanionMode)
}

// =============================================================================
// buildMessageContext tests
// =============================================================================

func TestBuildMessageContext(t *testing.T) {
	p := newTestParticipant()
	mem := newTestMemory()
	channel := &channels.Channel{ChatMode: true}
	network := &networks.Network{Nick: "BirdBot"}

	p.recordMessage(mem, "alice", "hello", false)
	p.recordMessage(mem, "bob", "hi", false)

	event := newTestEvent("#test", "alice", "hey BirdBot")
	ctx := p.buildMessageContext(mem, channel, network, event, "reactive")

	assert.Equal(t, "reactive", ctx.Type)
	assert.True(t, ctx.IsHighlighted)
	assert.Equal(t, "alice", ctx.TriggerUser)
	assert.NotEmpty(t, ctx.RecentMessages)
}
