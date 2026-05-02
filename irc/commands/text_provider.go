package commands

import (
	"fmt"

	"aibird/logger"
	"aibird/settings"
	"aibird/text/glm"
	"aibird/text/llamacpp"
)

// hasTextProviderConfig returns true if at least one text AI provider is configured.
// This is a fast config-only check with no network calls, suitable for guard clauses.
func hasTextProviderConfig(config *settings.Config) bool {
	return config.LlamaCpp.Url != "" || config.Glm.ApiKey != ""
}

// singleRequestWithFallback tries LlamaCpp first, then falls back to GLM on failure
// or unavailability. systemPrompt may be empty for user-prompt-only requests.
//
// Note: llamacpp.SingleRequest takes (message, system) order, while
// glm.SingleRequestWithSystem takes (system, message) order. The argument swap
// is handled inside this function.
func singleRequestWithFallback(systemPrompt, userPrompt string, config *settings.Config) (string, error) {
	// Try LlamaCpp if configured — skip health check and attempt directly.
	// SingleRequest will return an error if the server is offline or the request fails.
	if config.LlamaCpp.Url != "" {
		// Note: llamacpp.SingleRequest takes (message, system) — swapped from our params
		result, reqErr := llamacpp.SingleRequest(userPrompt, systemPrompt, config.LlamaCpp)
		if reqErr == nil {
			return result, nil
		}
		logger.Warn("LlamaCpp request failed, falling back to GLM", "error", reqErr)

		// Fallback to GLM if configured
		if config.Glm.ApiKey != "" {
			return glm.SingleRequestWithSystem(systemPrompt, userPrompt, config.Glm)
		}

		// No GLM fallback available — return the original LlamaCpp error
		return "", fmt.Errorf("LlamaCpp failed and GLM not configured: %w", reqErr)
	}

	// LlamaCpp not configured — try GLM directly
	if config.Glm.ApiKey != "" {
		return glm.SingleRequestWithSystem(systemPrompt, userPrompt, config.Glm)
	}

	return "", fmt.Errorf("no AI provider available (LlamaCpp not configured and GLM not configured)")
}
