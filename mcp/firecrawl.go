package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aibird/helpers"
	"aibird/logger"
	"aibird/settings"
)

// maxScrapeChars limits scraped content to prevent context overflow.
const maxScrapeChars = 5000

// maxScrapeResponseBytes limits Firecrawl response body to 5MB.
const maxScrapeResponseBytes = 5 << 20

// firecrawlRequest is the request body for Firecrawl v1 scrape API.
type firecrawlRequest struct {
	URL string `json:"url"`
}

// firecrawlResponse is the response from Firecrawl v1 scrape API.
type firecrawlResponse struct {
	Success bool              `json:"success"`
	Data    firecrawlData     `json:"data"`
	Status  string            `json:"status"`
	Meta    map[string]string `json:"metadata,omitempty"`
}

// firecrawlData holds the scraped page content.
type firecrawlData struct {
	Markdown    string `json:"markdown"`
	HTML        string `json:"html"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ScrapeURL fetches and extracts content from a web page using Firecrawl.
// The target URL is validated for SSRF protection before making the request.
func ScrapeURL(ctx context.Context, targetURL string, config settings.FirecrawlConfig) (string, error) { //nolint:gocyclo // scrape pipeline inherently has many branches
	if config.Url == "" {
		return "", fmt.Errorf("firecrawl scrape: URL not configured")
	}

	// SSRF protection: validate target URL before fetching
	if err := validateURL(targetURL); err != nil {
		return "", fmt.Errorf("firecrawl scrape: URL validation failed: %w", err)
	}

	scrapeURL := helpers.MakeUrlWithPort(config.Url, config.Port) + "v1/scrape"

	reqBody := firecrawlRequest{URL: targetURL}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("firecrawl scrape: failed to marshal request: %w", err)
	}

	logger.Debug("Firecrawl scrape request", "url", scrapeURL, "target", targetURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, scrapeURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("firecrawl scrape: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if config.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+config.ApiKey)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("firecrawl scrape: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxScrapeResponseBytes))
	if err != nil {
		return "", fmt.Errorf("firecrawl scrape: failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("firecrawl scrape: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var fcResp firecrawlResponse
	if err := json.Unmarshal(body, &fcResp); err != nil {
		return "", fmt.Errorf("firecrawl scrape: failed to parse response: %w", err)
	}

	if !fcResp.Success {
		return "", fmt.Errorf("firecrawl scrape: API returned success=false, status=%s", fcResp.Status)
	}

	content := fcResp.Data.Markdown
	if content == "" {
		// Fallback to HTML if markdown is empty
		content = fcResp.Data.HTML
	}

	if content == "" {
		return "No content could be extracted from this page.", nil
	}

	// Truncate if too long
	if len(content) > maxScrapeChars {
		content = content[:maxScrapeChars] + "\n\n[Content truncated at " + fmt.Sprintf("%d", maxScrapeChars) + " characters]"
	}

	// Add page metadata if available
	var sb strings.Builder
	if fcResp.Data.Title != "" {
		sb.WriteString("# " + fcResp.Data.Title + "\n\n")
	}
	if fcResp.Data.Description != "" {
		sb.WriteString("> " + fcResp.Data.Description + "\n\n")
	}
	sb.WriteString(content)

	return sb.String(), nil
}
