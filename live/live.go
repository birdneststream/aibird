// Package live tracks announced YouTube live streams per IRC channel and,
// when a Checker is configured, polls for stream end so the channel topic can
// be flipped from ON AIR to WAS LIVE automatically.
package live

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"aibird/logger"
)

// Topic and message formats used across the on-air lifecycle.
const (
	// TopicOnAirFormat is the topic set when a stream starts (%s = stream URL).
	TopicOnAirFormat = "ON AIR 🔴 | BIRDNEST IS LIVE %s"
	// TopicWasLiveFormat is the topic set when a stream ends (%s = stream URL).
	TopicWasLiveFormat = "BIRDNEST | WAS LIVE %s"
	// EndMessage is sent to the channel when a stream is detected as finished.
	EndMessage = "🔴 Stream has ended — topic updated."
)

// DefaultPromoLines is the promo broadcast repeated in the main channel
// while a stream is active. Empty lines become single-space separator
// messages on IRC.
func DefaultPromoLines() []string {
	return []string{
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
}

const (
	defaultPollInterval  = 120 * time.Second
	minPollInterval      = 30 * time.Second
	maxConsecutiveErrors = 5

	// defaultPromoInterval is the fallback promo cadence.
	defaultPromoInterval = 30 * time.Minute
	// minPromoInterval guards against config typos spamming the channel.
	minPromoInterval = time.Minute
	// promoLinePace throttles consecutive promo lines for flood safety.
	promoLinePace = 300 * time.Millisecond
)

// Notifier abstracts IRC output so the manager can update a channel's topic
// and send messages without depending on a concrete IRC client.
type Notifier interface {
	SetTopic(channel, topic string)
	MessageChannel(channel, message string)
}

// Checker reports whether a YouTube video is currently broadcasting.
type Checker interface {
	IsLive(ctx context.Context, videoID string) (bool, error)
}

// Stream is a single announced live stream in one channel of one network.
type Stream struct {
	Network   string
	Channel   string
	VideoID   string
	URL       string
	StartedAt time.Time

	notifier      Notifier
	consecErrors  int
	notifiedError bool
}

// Manager tracks active streams keyed by "network|channel" and polls them
// for completion when a Checker is configured. It can also repeat a promo
// broadcast in a designated channel while that channel's stream is active.
type Manager struct {
	mu           sync.Mutex
	streams      map[string]*Stream
	checker      Checker
	pollInterval time.Duration

	// Promo state (immutable after EnablePromo except lastPromoAt).
	promoNetwork  string
	promoChannel  string
	promoInterval time.Duration
	promoLines    []string
	lastPromoAt   time.Time

	// now is swappable for tests.
	now func() time.Time
}

var (
	defaultMu  sync.RWMutex
	defaultMgr *Manager
)

// NewManager creates a Manager. A nil checker disables polling entirely
// (Run returns immediately) while Start/Stop keep working for manual use.
// The interval is normalized: <= 0 becomes the 120s default and values below
// 30s are clamped up to 30s to protect the YouTube API quota.
func NewManager(checker Checker, pollInterval time.Duration) *Manager {
	switch {
	case pollInterval <= 0:
		pollInterval = defaultPollInterval
	case pollInterval < minPollInterval:
		pollInterval = minPollInterval
	}

	return &Manager{
		streams:      make(map[string]*Stream),
		checker:      checker,
		pollInterval: pollInterval,
		now:          time.Now,
	}
}

// PromoConfig describes the periodic promo broadcast for an active stream.
type PromoConfig struct {
	Network  string
	Channel  string
	Interval time.Duration
	Lines    []string
}

// EnablePromo arms the periodic promo broadcast. While a stream started via
// Start is active in cfg.Network/cfg.Channel, the promo lines repeat every
// cfg.Interval. The interval is normalized: <= 0 becomes the 30m default and
// values below one minute clamp up to one minute.
//
// Contract: call once at startup, before Run starts, and never while Run is
// live — promo fields are read without the manager lock inside the Run loop
// (safe only because of the goroutine-start happens-before edge).
func (m *Manager) EnablePromo(cfg PromoConfig) {
	switch {
	case cfg.Interval <= 0:
		cfg.Interval = defaultPromoInterval
	case cfg.Interval < minPromoInterval:
		cfg.Interval = minPromoInterval
	}

	m.mu.Lock()
	m.promoNetwork = cfg.Network
	m.promoChannel = cfg.Channel
	m.promoInterval = cfg.Interval
	m.promoLines = cfg.Lines
	m.mu.Unlock()
}

// promoEnabled reports whether a promo target is configured.
func (m *Manager) promoEnabled() bool {
	return m != nil && m.promoNetwork != "" && m.promoChannel != "" && m.promoInterval > 0
}

// Init sets the package-level manager used by the commands package. A nil
// argument is ignored so an accidental Init(nil) can't disable a live manager.
func Init(m *Manager) {
	if m == nil {
		return
	}
	defaultMu.Lock()
	defaultMgr = m
	defaultMu.Unlock()
}

// Default returns the package-level manager, lazily creating a
// monitoring-disabled fallback so callers never have to nil-check.
func Default() *Manager {
	defaultMu.RLock()
	m := defaultMgr
	defaultMu.RUnlock()
	if m != nil {
		return m
	}

	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultMgr == nil {
		defaultMgr = NewManager(nil, 0)
	}
	return defaultMgr
}

// MonitoringEnabled reports whether stream-end polling is active.
func (m *Manager) MonitoringEnabled() bool {
	return m != nil && m.checker != nil
}

// PollInterval returns the normalized polling interval.
func (m *Manager) PollInterval() time.Duration {
	return m.pollInterval
}

func streamKey(network, channel string) string {
	return network + "|" + channel
}

// Start marks a channel as on-air, replacing any existing stream for it, and
// sets the ON AIR topic immediately via the given notifier. The notifier is
// stored on the stream so Stop and the poller can later flip the topic on the
// correct per-network client.
func (m *Manager) Start(network, channel, videoID, url string, n Notifier) {
	// Defense-in-depth: never let raw CR/LF from pasted input reach a topic line.
	url = strings.NewReplacer("\r", "", "\n", "").Replace(url)

	key := streamKey(network, channel)
	s := &Stream{
		Network:   network,
		Channel:   channel,
		VideoID:   videoID,
		URL:       url,
		StartedAt: m.now(),
		notifier:  n,
	}

	m.mu.Lock()
	m.streams[key] = s
	// A fresh !live in the promo target channel restarts the promo cadence,
	// so the first promo lands a full interval into the new stream.
	if key == streamKey(m.promoNetwork, m.promoChannel) {
		m.lastPromoAt = s.StartedAt
	}
	m.mu.Unlock()

	if n != nil {
		n.SetTopic(channel, fmt.Sprintf(TopicOnAirFormat, url))
	}
}

// Stop ends the on-air state for a channel: it removes the stream and flips
// the topic to WAS LIVE using the notifier stored at Start time. It reports
// whether a stream was actually active.
func (m *Manager) Stop(network, channel string) bool {
	key := streamKey(network, channel)

	// Delete and flip the topic under the lock so a Start racing this Stop
	// can't land its ON AIR topic between the two. Notifier calls only
	// enqueue IRC writes, so the hold is cheap.
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.streams[key]
	if !ok {
		return false
	}

	if s.notifier != nil {
		s.notifier.SetTopic(channel, fmt.Sprintf(TopicWasLiveFormat, s.URL))
	}
	delete(m.streams, key)
	return true
}

// Status returns the active stream for a channel, if any.
func (m *Manager) Status(network, channel string) (Stream, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.streams[streamKey(network, channel)]
	if !ok {
		return Stream{}, false
	}
	return *s, true
}

// RunOnce polls every active stream a single time. Checker calls and
// notifications happen outside the manager lock so Start/Stop/Status from IRC
// handlers never block on network I/O.
func (m *Manager) RunOnce(ctx context.Context) {
	m.mu.Lock()
	snapshot := make([]*Stream, 0, len(m.streams))
	for _, s := range m.streams {
		snapshot = append(snapshot, s)
	}
	m.mu.Unlock()

	for _, s := range snapshot {
		if ctx.Err() != nil {
			return
		}
		m.pollStream(ctx, s)
	}
}

// pollStream checks one stream and finishes it when the broadcast has ended.
// Only the Run goroutine mutates the error counters, so no extra locking is
// needed for them.
func (m *Manager) pollStream(ctx context.Context, s *Stream) {
	isLive, err := m.checker.IsLive(ctx, s.VideoID)
	if err != nil {
		s.consecErrors++
		logger.Warn("Live stream poll failed",
			"network", s.Network,
			"channel", s.Channel,
			"video", s.VideoID,
			"consecutive_errors", s.consecErrors,
			"error", err)

		// A persistent API failure (bad key, quota) must never auto-flip the
		// topic — notify the channel once and keep the manual !live off path.
		if s.consecErrors >= maxConsecutiveErrors && !s.notifiedError {
			s.notifiedError = true
			if s.notifier != nil {
				s.notifier.MessageChannel(s.Channel,
					fmt.Sprintf("⚠️ Live monitor: repeated YouTube API errors (%d in a row) — auto topic update may be delayed; use !live off to end manually.", s.consecErrors))
			}
		}
		return
	}

	s.consecErrors = 0
	s.notifiedError = false

	if isLive {
		return
	}

	// Broadcast finished. Delete and notify under the lock — with a pointer
	// identity check — so a stream replaced by a newer !live while this poll
	// was in flight never gets its topic flipped or clobbers the new one,
	// and a Start racing the finish can't land between our delete and the
	// WAS LIVE flip. Notifier calls only enqueue IRC writes, so the hold is
	// cheap.
	m.mu.Lock()
	key := streamKey(s.Network, s.Channel)
	if cur, ok := m.streams[key]; ok && cur == s {
		if s.notifier != nil {
			s.notifier.SetTopic(s.Channel, fmt.Sprintf(TopicWasLiveFormat, s.URL))
			s.notifier.MessageChannel(s.Channel, EndMessage)
		}
		delete(m.streams, key)
	}
	m.mu.Unlock()
}

// promoPass sends the promo broadcast into the promo target channel when its
// stream is active and the interval has elapsed. Sends happen outside the
// lock; lastPromoAt is claimed under the lock first so a concurrent pass
// can't double-send.
func (m *Manager) promoPass() {
	if !m.promoEnabled() {
		return
	}

	m.mu.Lock()
	s := m.streams[streamKey(m.promoNetwork, m.promoChannel)]
	if s == nil || m.now().Sub(m.lastPromoAt) < m.promoInterval {
		m.mu.Unlock()
		return
	}
	m.lastPromoAt = m.now()
	notifier := s.notifier
	channel := s.Channel
	m.mu.Unlock()

	if notifier == nil {
		return
	}

	for _, line := range m.promoLines {
		if strings.TrimSpace(line) == "" {
			line = " " // blank separator line, mirroring State.Send behavior
		}
		notifier.MessageChannel(channel, line)
		time.Sleep(promoLinePace)
	}
}

// Run polls all active streams and runs the promo cadence until ctx is
// canceled, sleeping a full interval after each pass so cycles never
// overlap. It returns immediately when neither a checker nor a promo is
// configured.
func (m *Manager) Run(ctx context.Context) {
	if m.checker == nil && !m.promoEnabled() {
		logger.Info("Live monitor disabled: no YouTube API key or promo configured")
		return
	}

	logger.Info("Live monitor started", "poll_interval", m.pollInterval, "promo_enabled", m.promoEnabled())
	for {
		if m.checker != nil {
			m.RunOnce(ctx)
		}
		m.promoPass()

		select {
		case <-ctx.Done():
			logger.Info("Live monitor stopped")
			return
		case <-time.After(m.pollInterval):
		}
	}
}
