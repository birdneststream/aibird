package live

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// defaultYouTubeAPIBase is the YouTube Data API v3 videos.list endpoint.
const defaultYouTubeAPIBase = "https://www.googleapis.com/youtube/v3/videos"

// reVideoID matches YouTube video IDs: exactly 11 characters of the
// URL-safe base64 alphabet.
var reVideoID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// hostYouTube is the normalized YouTube host (www./m./music. stripped).
const hostYouTube = "youtube.com"

// YouTubeChecker implements Checker via the YouTube Data API v3
// videos.list endpoint.
//
// It intentionally uses its own context-aware net/http call instead of
// http/request.Request.Call: YouTube returns valid JSON error bodies
// (quota exhaustion, invalid key) with non-200 status codes, and those must
// surface as errors rather than decode to empty items and be mistaken for
// "stream ended".
type YouTubeChecker struct {
	apiKey  string
	BaseURL string // endpoint base URL; overridable for tests
	client  *http.Client
}

// NewYouTubeChecker creates a checker against the production API endpoint.
func NewYouTubeChecker(apiKey string) *YouTubeChecker {
	return &YouTubeChecker{
		apiKey:  apiKey,
		BaseURL: defaultYouTubeAPIBase,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// IsLive reports whether the video is currently broadcasting. An "upcoming"
// broadcast counts as live so a pre-announced stream doesn't immediately
// register as ended. A missing video (empty items), a completed broadcast
// (actualEndTime set), or ordinary non-live content counts as finished.
// Non-200 responses are errors, never "finished".
func (c *YouTubeChecker) IsLive(ctx context.Context, videoID string) (bool, error) {
	endpoint := fmt.Sprintf("%s?part=snippet,liveStreamingDetails&id=%s&key=%s",
		c.BaseURL, url.QueryEscape(videoID), url.QueryEscape(c.apiKey))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return false, fmt.Errorf("live check: build request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("live check: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("live check: youtube api returned status %d", resp.StatusCode)
	}

	var result videosResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("live check: decode response: %w", err)
	}

	if len(result.Items) == 0 {
		// Deleted, private, or nonexistent video: treat as finished, not error.
		return false, nil
	}

	item := result.Items[0]

	// liveStreamingDetails is a top-level item property (not under snippet).
	// An explicit end time takes precedence over a lagging content field.
	if item.LiveStreamingDetails.ActualEndTime != "" {
		return false, nil
	}

	switch item.Snippet.LiveBroadcastContent {
	case "live", "upcoming":
		return true, nil
	default:
		return false, nil
	}
}

// videosResponse models the subset of the videos.list response we consume.
type videosResponse struct {
	Items []videoItem `json:"items"`
}

type videoItem struct {
	Snippet              videoSnippet        `json:"snippet"`
	LiveStreamingDetails videoLiveStreamInfo `json:"liveStreamingDetails"`
}

type videoSnippet struct {
	LiveBroadcastContent string `json:"liveBroadcastContent"`
}

type videoLiveStreamInfo struct {
	ActualEndTime string `json:"actualEndTime"`
}

// ExtractVideoID parses a YouTube video ID from the common URL forms:
// watch?v=, youtu.be/<id>, /live/<id>, /shorts/<id>, /embed/<id>, /v/<id>,
// including m./music. subdomains and scheme-less pastes. Only proper URLs
// are accepted — a bare 11-character ID is rejected so an admin typo can't
// silently become a garbage announcement. Returns "" when no valid ID is found.
func ExtractVideoID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Normalize scheme-less pastes like "youtu.be/<id>" so url.Parse
	// populates Host instead of treating the whole string as a path.
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}

	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}

	host := strings.ToLower(u.Host)
	host = strings.TrimPrefix(host, "www.")
	if host == hostYouTube || strings.HasSuffix(host, "."+hostYouTube) {
		host = hostYouTube
	}

	if host == "youtu.be" {
		return validVideoID(strings.Trim(u.Path, "/"))
	}

	if host != hostYouTube {
		return ""
	}

	// watch?v=<id> — query parsing terminates the ID at '&' for us.
	if id := u.Query().Get("v"); id != "" {
		return validVideoID(id)
	}

	// Path forms: /live/<id>, /shorts/<id>, /embed/<id>, /v/<id>.
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 2 {
		switch parts[0] {
		case "live", "shorts", "embed", "v":
			return validVideoID(parts[1])
		}
	}

	return ""
}

// validVideoID returns the ID only if it matches the exact 11-character form.
func validVideoID(id string) string {
	if reVideoID.MatchString(id) {
		return id
	}
	return ""
}
