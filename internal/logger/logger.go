// Package logger provides structured logging for Bakery CLI.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

var global *slog.Logger

// Setup initializes the global slog logger with a TextHandler at the given level.
// Supported levels: "DEBUG", "INFO", "WARN", "ERROR" (case-insensitive).
func Setup(level string) {
	l := parseLevel(level)
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     l,
		AddSource: false,
	})
	global = slog.New(h)
}

// Get returns the global logger. Must call Setup() first.
func Get() *slog.Logger {
	if global == nil {
		Setup("INFO")
	}
	return global
}

// WithModule returns a sub-logger tagged with the given module name.
func WithModule(name string) *slog.Logger {
	return Get().With("module", name)
}

func parseLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
