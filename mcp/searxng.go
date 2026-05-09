package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aibird/helpers"
	"aibird/logger"
	"aibird/settings"
)

// maxSearchResults limits the number of search results returned to the LLM.
const maxSearchResults = 5

// maxSearchResponseBytes limits SearXNG response body to 1MB.
const maxSearchResponseBytes = 1 << 20

// searxngResult represents a single SearXNG search result.
type searxngResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"content"`
}

// searxngResponse represents the SearXNG JSON response format.
type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

// SearchWeb performs a web search using SearXNG and returns formatted results.
func SearchWeb(ctx context.Context, query string, config settings.SearXNGConfig) (string, error) {
	if config.Url == "" {
		return "", fmt.Errorf("searxng search: URL not configured")
	}

	searchURL := helpers.MakeUrlWithPort(config.Url, config.Port) + "search"
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "json")
	searchURL += "?" + params.Encode()

	logger.Debug("SearXNG search request", "url", searchURL, "query", query)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("searxng search: failed to create request: %w", err)
	}

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("searxng search: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("searxng search: unexpected status %d", resp.StatusCode)
	}

	// Limit response body size
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSearchResponseBytes))
	if err != nil {
		return "", fmt.Errorf("searxng search: failed to read response: %w", err)
	}

	var searchResp searxngResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return "", fmt.Errorf("searxng search: failed to parse response: %w", err)
	}

	if len(searchResp.Results) == 0 {
		return "No search results found.", nil
	}

	// Format results as markdown
	var sb strings.Builder
	limit := len(searchResp.Results)
	if limit > maxSearchResults {
		limit = maxSearchResults
	}

	for i := 0; i < limit; i++ {
		r := searchResp.Results[i]
		fmt.Fprintf(&sb, "### %d. %s\n", i+1, r.Title)
		fmt.Fprintf(&sb, "URL: %s\n", r.URL)
		if r.Description != "" {
			fmt.Fprintf(&sb, "%s\n", r.Description)
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}
