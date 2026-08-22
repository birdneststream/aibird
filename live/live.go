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

const (
	defaultPollInterval  = 120 * time.Second
	minPollInterval      = 30 * time.Second
	maxConsecutiveErrors = 5
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
// for completion when a Checker is configured.
type Manager struct {
	mu           sync.Mutex
	streams      map[string]*Stream
	checker      Checker
	pollInterval time.Duration
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
	}
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
		StartedAt: time.Now(),
		notifier:  n,
	}

	m.mu.Lock()
	m.streams[key] = s
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

// Run polls all active streams until ctx is canceled, sleeping a full
// interval after each pass so cycles never overlap. It returns immediately
// when no checker is configured.
func (m *Manager) Run(ctx context.Context) {
	if m.checker == nil {
		logger.Info("Live monitor disabled: no YouTube API key configured")
		return
	}

	logger.Info("Live monitor started", "poll_interval", m.pollInterval)
	for {
		m.RunOnce(ctx)

		select {
		case <-ctx.Done():
			logger.Info("Live monitor stopped")
			return
		case <-time.After(m.pollInterval):
		}
	}
}
