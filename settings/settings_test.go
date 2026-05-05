package settings

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTempConfig creates a temporary config.toml in dir.
func writeTempConfig(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0644))
}

func TestLoadConfig_MissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config file not found")
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	writeTempConfig(t, tmpDir, "this is not valid [[[[TOML")

	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error parsing config file")
}

func TestLoadConfig_ValidConfig(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))

	configContent := `
[networks.testnet]
networkName = "testnet"
nick = "testbot"
server = "irc.test.com"
port = 6667
ssl = false

[[networks.testnet.channels]]
name = "#test"

[aibird]
actionTrigger = "!"

[llamacpp]
url = "http://localhost:8080"
port = "8080"

[comfyui]
url = "http://localhost:8188"
port = 8188

[birdhole]
host = "http://localhost"
port = "3000"
endPoint = "/upload"
key = "secret"

[logging]
level = "info"
format = "text"
`
	writeTempConfig(t, tmpDir, configContent)

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.NotNil(t, cfg)
	assert.Equal(t, "!", cfg.AiBird.ActionTrigger)
}

func TestLoadConfig_DefaultVideoFalse(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))

	// When video is not specified in TOML, it defaults to false (zero value).
	configContent := `
[networks.testnet]
networkName = "testnet"
nick = "testbot"
server = "irc.test.com"
port = 6667
ssl = false

[[networks.testnet.channels]]
name = "#test"

[aibird]
actionTrigger = "!"

[llamacpp]
url = "http://localhost:8080"
port = "8080"

[comfyui]
url = "http://localhost:8188"
port = 8188

[birdhole]
host = "http://localhost"
port = "3000"
endPoint = "/upload"
key = "secret"

[logging]
level = "info"
format = "text"
`
	writeTempConfig(t, tmpDir, configContent)

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Networks["testnet"].Channels, 1)
	// Video defaults to false when not specified in config
	assert.False(t, cfg.Networks["testnet"].Channels[0].Video)
}

func TestLoadConfig_ExplicitVideoTrue(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))

	// When video = true is explicitly set, it should be respected.
	configContent := `
[networks.testnet]
networkName = "testnet"
nick = "testbot"
server = "irc.test.com"
port = 6667
ssl = false

[[networks.testnet.channels]]
name = "#test"
video = true

[aibird]
actionTrigger = "!"

[llamacpp]
url = "http://localhost:8080"
port = "8080"

[comfyui]
url = "http://localhost:8188"
port = 8188

[birdhole]
host = "http://localhost"
port = "3000"
endPoint = "/upload"
key = "secret"

[logging]
level = "info"
format = "text"
`
	writeTempConfig(t, tmpDir, configContent)

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Networks["testnet"].Channels, 1)
	assert.True(t, cfg.Networks["testnet"].Channels[0].Video)
}

func TestLoadConfig_ServiceConfigs(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))

	configContent := `
[networks.testnet]
networkName = "testnet"
nick = "testbot"
server = "irc.test.com"
port = 6667
ssl = false

[[networks.testnet.channels]]
name = "#test"

[aibird]
actionTrigger = "!"

[llamacpp]
url = "http://localhost:8080"
port = "8080"

[comfyui]
url = "http://localhost:8188"
port = 8188

[birdhole]
host = "http://localhost"
port = "3000"
endPoint = "/upload"
key = "secret"

[logging]
level = "info"
format = "text"
`
	writeTempConfig(t, tmpDir, configContent)

	// Create settings directory and a service config
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "settings"), 0755))
	glmContent := `
url = "https://api.glm.com"
apiKey = "test-key"
defaultModel = "glm-4"
`
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "settings", "glm.toml"), []byte(glmContent), 0644))

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://api.glm.com", cfg.Glm.Url)
	assert.Equal(t, "test-key", cfg.Glm.ApiKey)
	assert.Equal(t, "glm-4", cfg.Glm.DefaultModel)
}

func TestValidate_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Networks")
}
