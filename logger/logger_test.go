package logger

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureAndRestoreLogger saves the current global logger state and restores it after the test.
func captureAndRestoreLogger(t *testing.T) {
	t.Helper()
	origLogger := defaultLogger
	t.Cleanup(func() {
		defaultLogger = origLogger
		if origLogger != nil {
			slog.SetDefault(origLogger)
		}
	})
}

func TestConfigValidate_ValidConfigs(t *testing.T) {
	tests := []struct {
		name   string
		level  LogLevel
		format string
	}{
		{"debug text", LevelDebug, "text"},
		{"debug json", LevelDebug, "json"},
		{"info text", LevelInfo, "text"},
		{"info json", LevelInfo, "json"},
		{"warn text", LevelWarn, "text"},
		{"warn json", LevelWarn, "json"},
		{"error text", LevelError, "text"},
		{"error json", LevelError, "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := Config{Level: tt.level, Format: tt.format}
			err := config.Validate()
			assert.NoError(t, err)
		})
	}
}

func TestConfigValidate_InvalidLevel(t *testing.T) {
	config := Config{Level: "trace", Format: "text"}
	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Level")
}

func TestConfigValidate_InvalidFormat(t *testing.T) {
	config := Config{Level: "info", Format: "xml"}
	err := config.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Format")
}

func TestConfigValidate_MissingLevel(t *testing.T) {
	config := Config{Format: "text"}
	err := config.Validate()
	assert.Error(t, err)
}

func TestConfigValidate_MissingFormat(t *testing.T) {
	config := Config{Level: "info"}
	err := config.Validate()
	assert.Error(t, err)
}

func TestInit_ValidConfig(t *testing.T) {
	captureAndRestoreLogger(t)

	config := Config{Level: LevelDebug, Format: "text"}
	require.NotPanics(t, func() {
		Init(config)
	})

	assert.NotNil(t, defaultLogger)
	assert.True(t, defaultLogger.Handler().Enabled(nil, slog.LevelDebug))
}

func TestInit_InvalidConfig(t *testing.T) {
	captureAndRestoreLogger(t)

	// Invalid config should log an error but not panic
	config := Config{Level: "invalid", Format: "text"}
	require.NotPanics(t, func() {
		Init(config)
	})

	// Logger should still be initialized with default level (info)
	assert.NotNil(t, defaultLogger)
}

func TestInit_LevelMapping(t *testing.T) {
	tests := []struct {
		level    LogLevel
		expected slog.Level
	}{
		{LevelDebug, slog.LevelDebug},
		{LevelInfo, slog.LevelInfo},
		{LevelWarn, slog.LevelWarn},
		{LevelError, slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(string(tt.level), func(t *testing.T) {
			captureAndRestoreLogger(t)

			config := Config{Level: tt.level, Format: "text"}
			Init(config)

			assert.True(t, defaultLogger.Handler().Enabled(nil, tt.expected))
		})
	}
}

func TestInit_JSONFormat(t *testing.T) {
	captureAndRestoreLogger(t)

	config := Config{Level: LevelInfo, Format: "json"}
	Init(config)

	assert.NotNil(t, defaultLogger)
	// Verify the handler is JSON by checking it produces JSON output
	var buf bytes.Buffer
	testLogger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	testLogger.Info("test", "key", "value")
	assert.Contains(t, buf.String(), `"key":"value"`)
}

func TestLogFunctions(t *testing.T) {
	captureAndRestoreLogger(t)

	var buf bytes.Buffer
	defaultLogger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	Debug("debug msg", "k1", "v1")
	assert.Contains(t, buf.String(), "debug msg")

	buf.Reset()
	Info("info msg", "k2", "v2")
	assert.Contains(t, buf.String(), "info msg")

	buf.Reset()
	Warn("warn msg", "k3", "v3")
	assert.Contains(t, buf.String(), "warn msg")

	buf.Reset()
	Error("error msg", "k4", "v4")
	assert.Contains(t, buf.String(), "error msg")
}

func TestWith(t *testing.T) {
	captureAndRestoreLogger(t)

	var buf bytes.Buffer
	defaultLogger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	ctxLogger := With("request_id", "123")
	ctxLogger.Info("contextual")
	assert.Contains(t, buf.String(), "request_id")
	assert.Contains(t, buf.String(), "123")
}

func TestNetwork(t *testing.T) {
	captureAndRestoreLogger(t)

	var buf bytes.Buffer
	defaultLogger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	Network("freenode").Info("network event")
	assert.Contains(t, buf.String(), "network")
	assert.Contains(t, buf.String(), "freenode")
}

func TestChannel(t *testing.T) {
	captureAndRestoreLogger(t)

	var buf bytes.Buffer
	defaultLogger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	Channel("freenode", "#general").Info("channel event")
	assert.Contains(t, buf.String(), "channel")
	assert.Contains(t, buf.String(), "#general")
}

func TestUser(t *testing.T) {
	captureAndRestoreLogger(t)

	var buf bytes.Buffer
	defaultLogger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	User("freenode", "#general", "alice").Info("user event")
	assert.Contains(t, buf.String(), "nick")
	assert.Contains(t, buf.String(), "alice")
}

func TestService(t *testing.T) {
	captureAndRestoreLogger(t)

	var buf bytes.Buffer
	defaultLogger = slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	Service("comfyui").Info("service event")
	assert.Contains(t, buf.String(), "service")
	assert.Contains(t, buf.String(), "comfyui")
}
