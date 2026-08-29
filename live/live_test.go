package live

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeNotifier records topics and messages per channel.
type fakeNotifier struct {
	topics   map[string]string
	messages []string
}

func newFakeNotifier() *fakeNotifier {
	return &fakeNotifier{topics: make(map[string]string)}
}

func (f *fakeNotifier) SetTopic(channel, topic string) {
	f.topics[channel] = topic
}

func (f *fakeNotifier) MessageChannel(channel, message string) {
	f.messages = append(f.messages, message)
}

// fakeChecker returns a canned result or error.
type fakeChecker struct {
	result bool
	err    error
	calls  []string
	// onCall, when set, runs before returning (used to mutate manager state
	// mid-poll for race-condition tests).
	onCall func(videoID string)
}

func (f *fakeChecker) IsLive(_ context.Context, videoID string) (bool, error) {
	f.calls = append(f.calls, videoID)
	if f.onCall != nil {
		f.onCall(videoID)
	}
	return f.result, f.err
}

func newTestManager(c Checker) (*Manager, *fakeNotifier) {
	n := newFakeNotifier()
	return NewManager(c, 0), n
}

func TestNewManager_IntervalNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{"zero uses default", 0, defaultPollInterval},
		{"negative uses default", -1, defaultPollInterval},
		{"small clamps to minimum", 10 * time.Second, minPollInterval},
		{"exact minimum kept", 30 * time.Second, 30 * time.Second},
		{"explicit value kept", 5 * time.Minute, 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(nil, tt.input)
			assert.Equal(t, tt.expected, m.PollInterval())
		})
	}
}

func TestDefault_NonNilWithoutInit(t *testing.T) {
	// Default must never return nil, even before Init is called by main.
	assert.NotNil(t, Default())
	assert.False(t, Default().MonitoringEnabled())
}

func TestManager_StartSetsOnAirTopicAndStatus(t *testing.T) {
	m, n := newTestManager(nil)

	m.Start("supernets", "#h4x", "dQw4w9WgXcQ", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", n)

	assert.Equal(t, "ON AIR 🔴 | BIRDNEST IS LIVE https://www.youtube.com/watch?v=dQw4w9WgXcQ", n.topics["#h4x"])

	s, ok := m.Status("supernets", "#h4x")
	require.True(t, ok, "stream should be active after Start")
	assert.Equal(t, "dQw4w9WgXcQ", s.VideoID)
	assert.Equal(t, "https://www.youtube.com/watch?v=dQw4w9WgXcQ", s.URL)
	assert.False(t, s.StartedAt.IsZero(), "StartedAt should be set")
}

func TestManager_StartReplacesExistingStream(t *testing.T) {
	m, n := newTestManager(nil)

	m.Start("supernets", "#h4x", "aaaaaaaaaaa", "https://www.youtube.com/watch?v=aaaaaaaaaaa", n)
	m.Start("supernets", "#h4x", "bbbbbbbbbbb", "https://www.youtube.com/watch?v=bbbbbbbbbbb", n)

	s, ok := m.Status("supernets", "#h4x")
	require.True(t, ok)
	assert.Equal(t, "bbbbbbbbbbb", s.VideoID, "latest Start should win")

	// Only one stream tracked for the key.
	m.mu.Lock()
	count := len(m.streams)
	m.mu.Unlock()
	assert.Equal(t, 1, count)
}

func TestManager_StopFlipsTopicAndReports(t *testing.T) {
	m, n := newTestManager(nil)
	m.Start("supernets", "#h4x", "dQw4w9WgXcQ", "https://youtu.be/dQw4w9WgXcQ", n)

	assert.True(t, m.Stop("supernets", "#h4x"))
	assert.Equal(t, "BIRDNEST | WAS LIVE https://youtu.be/dQw4w9WgXcQ", n.topics["#h4x"])

	_, ok := m.Status("supernets", "#h4x")
	assert.False(t, ok, "stream should be removed after Stop")

	// Second stop reports no active stream and leaves the topic alone.
	assert.False(t, m.Stop("supernets", "#h4x"))
	assert.Equal(t, "BIRDNEST | WAS LIVE https://youtu.be/dQw4w9WgXcQ", n.topics["#h4x"])
}

func TestManager_StopUnknownKeyIsFalse(t *testing.T) {
	m, _ := newTestManager(nil)
	assert.False(t, m.Stop("nowhere", "#nope"))
}

func TestManager_StatusSeparatesNetworks(t *testing.T) {
	m, n := newTestManager(nil)
	m.Start("supernets", "#h4x", "aaaaaaaaaaa", "url-a", n)
	m.Start("efnet", "#h4x", "bbbbbbbbbbb", "url-b", n)

	sA, okA := m.Status("supernets", "#h4x")
	sB, okB := m.Status("efnet", "#h4x")
	require.True(t, okA)
	require.True(t, okB)
	assert.Equal(t, "aaaaaaaaaaa", sA.VideoID)
	assert.Equal(t, "bbbbbbbbbbb", sB.VideoID)

	// Stopping one network must not affect the other.
	assert.True(t, m.Stop("supernets", "#h4x"))
	_, okB2 := m.Status("efnet", "#h4x")
	assert.True(t, okB2)
}

func TestManager_RunOnce_FinishedStreamFlipsTopic(t *testing.T) {
	c := &fakeChecker{result: false}
	m, n := newTestManager(c)
	m.Start("supernets", "#h4x", "dQw4w9WgXcQ", "https://youtu.be/dQw4w9WgXcQ", n)

	m.RunOnce(context.Background())

	assert.Equal(t, "BIRDNEST | WAS LIVE https://youtu.be/dQw4w9WgXcQ", n.topics["#h4x"])
	assert.NotEmpty(t, n.messages, "an end message should be announced")
	_, ok := m.Status("supernets", "#h4x")
	assert.False(t, ok, "finished stream should be removed")
}

func TestManager_RunOnce_StillLiveKeepsStream(t *testing.T) {
	c := &fakeChecker{result: true}
	m, n := newTestManager(c)
	m.Start("supernets", "#h4x", "dQw4w9WgXcQ", "https://youtu.be/dQw4w9WgXcQ", n)

	m.RunOnce(context.Background())

	assert.Equal(t, "ON AIR 🔴 | BIRDNEST IS LIVE https://youtu.be/dQw4w9WgXcQ", n.topics["#h4x"])
	assert.Empty(t, n.messages)
	_, ok := m.Status("supernets", "#h4x")
	assert.True(t, ok)
}

func TestManager_RunOnce_ErrorKeepsStreamAndNeverFlipsTopic(t *testing.T) {
	c := &fakeChecker{err: errors.New("403 quota exceeded")}
	m, n := newTestManager(c)
	m.Start("supernets", "#h4x", "dQw4w9WgXcQ", "https://youtu.be/dQw4w9WgXcQ", n)

	m.RunOnce(context.Background())

	assert.Equal(t, "ON AIR 🔴 | BIRDNEST IS LIVE https://youtu.be/dQw4w9WgXcQ", n.topics["#h4x"], "API errors must never flip the topic")
	assert.Empty(t, n.messages, "no channel notice before the consecutive-error threshold")
	_, ok := m.Status("supernets", "#h4x")
	assert.True(t, ok, "stream must survive checker errors")
}

func TestManager_RunOnce_PersistentErrorsNotifyOnce(t *testing.T) {
	c := &fakeChecker{err: errors.New("401 invalid key")}
	m, n := newTestManager(c)
	m.Start("supernets", "#h4x", "dQw4w9WgXcQ", "https://youtu.be/dQw4w9WgXcQ", n)

	for range maxConsecutiveErrors + 1 {
		m.RunOnce(context.Background())
	}

	notices := 0
	for _, msg := range n.messages {
		if strings.Contains(msg, "repeated YouTube API errors") {
			notices++
		}
	}
	assert.Equal(t, 1, notices, "channel should be warned exactly once about persistent errors")

	_, ok := m.Status("supernets", "#h4x")
	assert.True(t, ok, "stream must still survive persistent errors")

	// A subsequent success resets the counter so a future failure streak
	// can notify again.
	c.err = nil
	c.result = true
	m.RunOnce(context.Background())
	c.err = errors.New("boom")
	for range maxConsecutiveErrors {
		m.RunOnce(context.Background())
	}
	notices = 0
	for _, msg := range n.messages {
		if strings.Contains(msg, "repeated YouTube API errors") {
			notices++
		}
	}
	assert.Equal(t, 2, notices, "notice should fire again after a fresh error streak")
}

func TestManager_RunOnce_ReplacedStreamNotFinished(t *testing.T) {
	// The checker replaces the stream mid-poll (as if the admin re-ran
	// !live with a new URL while a poll was in flight). The in-flight poll
	// must not flip the topic or delete the replacement.
	m, n := newTestManager(nil)
	m.Start("supernets", "#h4x", "aaaaaaaaaaa", "https://youtu.be/aaaaaaaaaaa", n)

	c := &fakeChecker{result: false}
	c.onCall = func(string) {
		m.Start("supernets", "#h4x", "bbbbbbbbbbb", "https://youtu.be/bbbbbbbbbbb", n)
	}
	m.checker = c

	m.RunOnce(context.Background())

	assert.Equal(t, "ON AIR 🔴 | BIRDNEST IS LIVE https://youtu.be/bbbbbbbbbbb", n.topics["#h4x"], "old stream's end must not clobber the new ON AIR topic")
	s, ok := m.Status("supernets", "#h4x")
	require.True(t, ok, "replacement stream must survive")
	assert.Equal(t, "bbbbbbbbbbb", s.VideoID)
}

func TestManager_RunOnce_HonorsCancelledContext(t *testing.T) {
	c := &fakeChecker{result: true}
	m, _ := newTestManager(c)
	m.Start("supernets", "#h4x", "aaaaaaaaaaa", "url", newFakeNotifier())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m.RunOnce(ctx)

	assert.Empty(t, c.calls, "no checker calls should happen with a canceled context")
}

func TestManager_MonitoringEnabled(t *testing.T) {
	withChecker := NewManager(&fakeChecker{}, 0)
	assert.True(t, withChecker.MonitoringEnabled())

	withoutChecker := NewManager(nil, 0)
	assert.False(t, withoutChecker.MonitoringEnabled())
}

func TestManager_Run_ReturnsImmediatelyWithoutCheckerOrPromo(t *testing.T) {
	m := NewManager(nil, 0)
	done := make(chan struct{})
	go func() {
		m.Run(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// Expected: Run exits immediately when monitoring and promo are disabled.
	case <-time.After(2 * time.Second):
		t.Fatal("Run should return immediately when no checker or promo is configured")
	}
}

// newPromoTestManager builds a manager with a promo target and an injectable
// clock so cadence tests never sleep.
func newPromoTestManager(interval time.Duration) (*Manager, *fakeNotifier) {
	n := newFakeNotifier()
	m := NewManager(nil, 0)
	m.EnablePromo(PromoConfig{
		Network:  "efnet",
		Channel:  "#birdnest",
		Interval: interval,
		Lines:    []string{"L1", "", "L2"},
	})
	return m, n
}

func TestManager_Promo_Cadence(t *testing.T) {
	m, n := newPromoTestManager(30 * time.Minute)
	cur := time.Now()
	m.now = func() time.Time { return cur }

	m.Start("efnet", "#birdnest", "aaaaaaaaaaa", "https://youtu.be/aaaaaaaaaaa", n)

	// Not due immediately: the !live announcement burst just happened.
	m.promoPass()
	assert.Empty(t, n.messages, "no promo before the interval elapses")

	// Due after the interval.
	cur = cur.Add(31 * time.Minute)
	m.promoPass()
	assert.Equal(t, []string{"L1", " ", "L2"}, n.messages, "promo lines should be sent with blank lines as separators")

	// Suppressed until the next interval.
	n.messages = nil
	m.promoPass()
	assert.Empty(t, n.messages, "second pass within the interval must be suppressed")

	cur = cur.Add(30 * time.Minute)
	m.promoPass()
	assert.Equal(t, []string{"L1", " ", "L2"}, n.messages)
}

func TestManager_Promo_RequiresTargetStream(t *testing.T) {
	m, n := newPromoTestManager(30 * time.Minute)
	cur := time.Now()
	m.now = func() time.Time { return cur }

	// Stream active, but in a different channel than the promo target.
	m.Start("supernets", "#h4x", "aaaaaaaaaaa", "https://youtu.be/aaaaaaaaaaa", n)
	cur = cur.Add(2 * time.Hour)
	m.promoPass()
	assert.Empty(t, n.messages, "promo must only target its configured channel")

	// And a stream on the right network but wrong channel.
	n.messages = nil
	m.Start("efnet", "#other", "bbbbbbbbbbb", "https://youtu.be/bbbbbbbbbbb", n)
	cur = cur.Add(2 * time.Hour)
	m.promoPass()
	assert.Empty(t, n.messages)
}

func TestManager_Promo_StopsWhenStreamEnds(t *testing.T) {
	m, n := newPromoTestManager(30 * time.Minute)
	cur := time.Now()
	m.now = func() time.Time { return cur }

	m.Start("efnet", "#birdnest", "aaaaaaaaaaa", "https://youtu.be/aaaaaaaaaaa", n)
	cur = cur.Add(31 * time.Minute)
	m.promoPass()
	assert.NotEmpty(t, n.messages)

	// Stream goes off air: no further promos.
	n.messages = nil
	m.Stop("efnet", "#birdnest")
	cur = cur.Add(2 * time.Hour)
	m.promoPass()
	assert.Empty(t, n.messages, "promo must stop once the stream ends")
}

func TestManager_Promo_RestartResetsCadence(t *testing.T) {
	m, n := newPromoTestManager(30 * time.Minute)
	cur := time.Now()
	m.now = func() time.Time { return cur }

	m.Start("efnet", "#birdnest", "aaaaaaaaaaa", "https://youtu.be/aaaaaaaaaaa", n)
	cur = cur.Add(20 * time.Minute)

	// Admin re-runs !live with a new URL: cadence restarts.
	m.Start("efnet", "#birdnest", "bbbbbbbbbbb", "https://youtu.be/bbbbbbbbbbb", n)
	cur = cur.Add(25 * time.Minute)
	m.promoPass()
	assert.Empty(t, n.messages, "promo must be a full interval after the new !live")

	cur = cur.Add(6 * time.Minute)
	m.promoPass()
	assert.Equal(t, []string{"L1", " ", "L2"}, n.messages)
}

func TestEnablePromo_IntervalNormalization(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected time.Duration
	}{
		{"zero uses default", 0, defaultPromoInterval},
		{"negative uses default", -1, defaultPromoInterval},
		{"sub-minute clamps up", 5 * time.Second, minPromoInterval},
		{"explicit value kept", time.Hour, time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewManager(nil, 0)
			m.EnablePromo(PromoConfig{Network: "efnet", Channel: "#birdnest", Interval: tt.input, Lines: nil})
			m.mu.Lock()
			got := m.promoInterval
			m.mu.Unlock()
			if got != tt.expected {
				t.Errorf("promo interval = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestManager_Promo_DisabledWithoutEnablePromo(t *testing.T) {
	m := NewManager(nil, 0)
	m.promoPass() // must be a no-op, not a panic
	assert.False(t, m.promoEnabled())
}

func TestManager_Run_ContinuesWithPromoOnly(t *testing.T) {
	// No YouTube checker, but a promo is configured: Run must keep looping
	// (guarding the "returns only when BOTH are absent" condition) and exit
	// cleanly on cancellation.
	m, n := newPromoTestManager(30 * time.Minute)
	m.Start("efnet", "#birdnest", "aaaaaaaaaaa", "https://youtu.be/aaaaaaaaaaa", n)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Run must keep looping when a promo is configured even without a checker")
	case <-time.After(200 * time.Millisecond):
		// Still running as expected.
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run must exit after context cancellation")
	}
}

func TestDefaultPromoLines(t *testing.T) {
	lines := DefaultPromoLines()
	if len(lines) != 11 {
		t.Fatalf("expected 11 promo lines, got %d", len(lines))
	}
	want := []string{
		"HAVE YOUR MESSAGE APPEAR ON THE STREAM LIVE",
		"WITH TEXT TO SPEECH:",
		"https://streamlabs.com/birdnestshow tip",
		"",
		"[PATREON] https://www.patreon.com/birdneststream",
		"",
		"[YT] https://www.youtube.com/@birdneststream",
		"",
		"[GITHUB] https://github.com/birdneststream/",
		"",
		"[SOUNDCLOUD] https://soundcloud.com/birdneststream",
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("promo line %d = %q, want %q", i, lines[i], w)
		}
	}
}
