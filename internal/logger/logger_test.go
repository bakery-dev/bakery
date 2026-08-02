package logger

import (
	"log/slog"
	"testing"
)

func TestSetupAndGet(t *testing.T) {
	Setup("DEBUG")
	l := Get()
	if l == nil {
		t.Fatal("Get() returned nil logger")
	}

	sub := WithModule("test-module")
	if sub == nil {
		t.Fatal("WithModule() returned nil logger")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"WARN", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"UNKNOWN", slog.LevelInfo},
	}

	for _, tt := range tests {
		got := parseLevel(tt.input)
		if got != tt.want {
			t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
