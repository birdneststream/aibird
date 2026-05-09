package mcp

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"aibird/settings"
)

func TestValidateURL_ValidPublicURLs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"https URL", "https://example.com/page"},
		{"http URL", "http://example.com/page"},
		{"URL with path", "https://example.com/docs/api/v1"},
		{"URL with query", "https://example.com/search?q=test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: example.com resolves to a real IP (93.184.216.34), so this should pass
			// If DNS is not available, this test may fail — that's acceptable for CI
			err := validateURL(tt.url)
			if err != nil {
				// Allow DNS resolution failures in test environments
				if isDNSError(err) {
					t.Skipf("DNS not available: %v", err)
				}
				t.Errorf("validateURL(%q) returned unexpected error: %v", tt.url, err)
			}
		})
	}
}

func TestValidateURL_BlockedSchemes(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		errMsg string
	}{
		{"file scheme", "file:///etc/passwd", "unsupported scheme"},
		{"ftp scheme", "ftp://example.com/file", "unsupported scheme"},
		{"javascript scheme", "javascript:alert(1)", "unsupported scheme"},
		{"data scheme", "data:text/html,<h1>test</h1>", "unsupported scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if err == nil {
				t.Errorf("expected error for %q, got nil", tt.url)
			} else if !contains(err.Error(), tt.errMsg) {
				t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
			}
		})
	}
}

func TestValidateURL_BlockedIPs(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"loopback", "http://127.0.0.1/admin"},
		{"localhost", "http://localhost:8080/metrics"},
		{"private A", "http://10.0.0.1/secret"},
		{"private B", "http://172.16.0.1/secret"},
		{"private C", "http://192.168.1.1/secret"},
		{"link local", "http://169.254.1.1/secret"},
		{"unspecified", "http://0.0.0.0/secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateURL(tt.url)
			if err == nil {
				t.Errorf("expected error for %q, got nil", tt.url)
			}
		})
	}
}

func TestValidateURL_EmptyHost(t *testing.T) {
	err := validateURL("http:///path")
	if err == nil {
		t.Error("expected error for empty host, got nil")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || sub == "" ||
		(s != "" && sub != "" && findSubstring(s, sub)))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func isDNSError(err error) bool {
	if dnsErr, ok := err.(*net.DNSError); ok {
		_ = dnsErr
		return true
	}
	return false
}

func TestSearchWeb_Success(t *testing.T) {
	mockResp := `{
		"results": [
			{"title": "Go 1.25 Release", "url": "https://go.dev/blog/go1.25", "content": "Go 1.25 is released with new features."},
			{"title": "Go Programming", "url": "https://go.dev/", "content": "The Go programming language."}
		]
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "json" {
			t.Error("expected format=json parameter")
		}
		if r.URL.Query().Get("q") == "" {
			t.Error("expected q parameter")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockResp)) //nolint:errcheck
	}))
	defer server.Close()

	result, err := SearchWeb(t.Context(), "Go 1.25", testSearXNGConfig(server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	if !contains(result, "Go 1.25 Release") {
		t.Error("expected title in result")
	}
	if !contains(result, "https://go.dev/blog/go1.25") {
		t.Error("expected URL in result")
	}
}

func TestSearchWeb_EmptyResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"results": []}`)) //nolint:errcheck
	}))
	defer server.Close()

	result, err := SearchWeb(t.Context(), "obscure query", testSearXNGConfig(server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "No search results found." {
		t.Errorf("expected 'No search results found.', got %q", result)
	}
}

func TestSearchWeb_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := SearchWeb(t.Context(), "test", testSearXNGConfig(server.URL))
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestSearchWeb_Unconfigured(t *testing.T) {
	_, err := SearchWeb(t.Context(), "test", testSearXNGConfigEmpty())
	if err == nil {
		t.Error("expected error when URL not configured")
	}
}

func TestScrapeURL_Success(t *testing.T) {
	mockResp := `{
		"success": true,
		"data": {
			"markdown": "# Page Title\n\nThis is the page content with detailed information.",
			"html": "",
			"title": "Test Page",
			"description": "A test page description"
		}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected Content-Type: application/json")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockResp)) //nolint:errcheck
	}))
	defer server.Close()

	// Use example.com as target (public IP)
	result, err := ScrapeURL(t.Context(), "https://example.com", testFirecrawlConfig(server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	if !contains(result, "Test Page") {
		t.Error("expected title in result")
	}
	if !contains(result, "page content") {
		t.Error("expected content in result")
	}
}

func TestScrapeURL_Truncation(t *testing.T) {
	longContent := make([]byte, maxScrapeChars+1000)
	for i := range longContent {
		longContent[i] = 'a'
	}

	mockResp := `{"success":true,"data":{"markdown":"` + string(longContent) + `"}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(mockResp)) //nolint:errcheck
	}))
	defer server.Close()

	result, err := ScrapeURL(t.Context(), "https://example.com", testFirecrawlConfig(server.URL))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) > maxScrapeChars+500 { // allow for metadata + truncation message
		t.Errorf("result too long: %d chars (limit %d)", len(result), maxScrapeChars)
	}
	if !contains(result, "truncated") {
		t.Error("expected truncation notice")
	}
}

func TestScrapeURL_SSRFBlocked(t *testing.T) {
	// Should block localhost
	_, err := ScrapeURL(t.Context(), "http://127.0.0.1:8080/admin", testFirecrawlConfig("http://firecrawl:3000"))
	if err == nil {
		t.Error("expected SSRF validation error for localhost")
	}
}

func TestScrapeURL_Unconfigured(t *testing.T) {
	_, err := ScrapeURL(t.Context(), "https://example.com", testFirecrawlConfigEmpty())
	if err == nil {
		t.Error("expected error when URL not configured")
	}
}

func TestScrapeURL_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success":false,"status":"error"}`)) //nolint:errcheck
	}))
	defer server.Close()

	_, err := ScrapeURL(t.Context(), "https://example.com", testFirecrawlConfig(server.URL))
	if err == nil {
		t.Error("expected error for success=false response")
	}
}

// Test helpers

func testSearXNGConfig(serverURL string) settings.SearXNGConfig {
	return settings.SearXNGConfig{Url: serverURL, Port: ""}
}

func testSearXNGConfigEmpty() settings.SearXNGConfig {
	return settings.SearXNGConfig{}
}

func testFirecrawlConfig(serverURL string) settings.FirecrawlConfig {
	return settings.FirecrawlConfig{Url: serverURL, Port: ""}
}

func testFirecrawlConfigEmpty() settings.FirecrawlConfig {
	return settings.FirecrawlConfig{}
}
