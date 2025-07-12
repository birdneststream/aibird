package settings

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/go-playground/validator/v10"
)

// Validate validates the loaded configuration
func (c *Config) Validate() error {
	validate := validator.New(validator.WithRequiredStructEnabled())
	return validate.Struct(c)
}

// LoadConfig loads the configuration from the config.toml file and all service configs.
// It returns a pointer to the Config struct or an error if loading fails.
func LoadConfig() (*Config, error) {
	var config Config
	configPath := "config.toml"

	// Check if main config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", configPath)
	}

	// Get absolute path for better error messages
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		absPath = configPath // fallback to relative path
	}

	_, err = toml.DecodeFile(configPath, &config)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file %s: %w", absPath, err)
	}

	// Load service-specific configs
	if err := loadServiceConfigs(&config); err != nil {
		return nil, fmt.Errorf("error loading service configs: %w", err)
	}

	// Validate the configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &config, nil
}

// loadServiceConfigs loads all individual service configuration files
func loadServiceConfigs(config *Config) error {
	serviceConfigs := map[string]interface{}{
		"settings/openrouter.toml": &config.OpenRouter,
		"settings/gemini.toml":     &config.Gemini,
		"settings/ollama.toml":     &config.Ollama,
		"settings/comfyui.toml":    &config.ComfyUi,
		"settings/birdhole.toml":   &config.Birdhole,
		"settings/logging.toml":    &config.Logging,
	}

	for configPath, configStruct := range serviceConfigs {
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			// This is not a fatal error, just a warning
			continue
		}

		_, err := toml.DecodeFile(configPath, configStruct)
		if err != nil {
			return fmt.Errorf("error parsing service config file %s: %w", configPath, err)
		}
	}

	return nil
}
