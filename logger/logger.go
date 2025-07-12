package logger

import (
	"context"
	"log/slog"
	"os"

	"github.com/go-playground/validator/v10"
)

var defaultLogger *slog.Logger
var validate *validator.Validate

// LogLevel represents log levels
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// Config holds logger configuration
type Config struct {
	Level  LogLevel `toml:"level" validate:"required,oneof=debug info warn error"`
	Format string   `toml:"format" validate:"required,oneof=text json"` // "text" or "json"
}

// Validate validates the logger configuration
func (c *Config) Validate() error {
	validate = validator.New(validator.WithRequiredStructEnabled())
	return validate.Struct(c)
}

// Init initializes the global logger with the given configuration
func Init(config Config) {
	if err := config.Validate(); err != nil {
		slog.Error("Invalid logger configuration", "error", err)
	}
	var level slog.Level
	switch config.Level {
	case LevelDebug:
		level = slog.LevelDebug
	case LevelInfo:
		level = slog.LevelInfo
	case LevelWarn:
		level = slog.LevelWarn
	case LevelError:
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{
		Level: level,
	}

	switch config.Format {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
}

// Debug logs at debug level
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

// Info logs at info level
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// Warn logs at warn level
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// Error logs at error level
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}

// With returns a logger with additional context
func With(args ...any) *slog.Logger {
	return defaultLogger.With(args...)
}

// WithContext returns a logger with context
func WithContext(ctx context.Context) *slog.Logger {
	return defaultLogger.With()
}

// Fatal logs an error and exits the program
func Fatal(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
	os.Exit(1)
}

// Network creates a logger with network context
func Network(name string) *slog.Logger {
	return defaultLogger.With("network", name)
}

// Channel creates a logger with channel context
func Channel(network, channel string) *slog.Logger {
	return defaultLogger.With("network", network, "channel", channel)
}

// User creates a logger with user context
func User(network, channel, nick string) *slog.Logger {
	return defaultLogger.With("network", network, "channel", channel, "nick", nick)
}

// Service creates a logger with service context
func Service(service string) *slog.Logger {
	return defaultLogger.With("service", service)
}
