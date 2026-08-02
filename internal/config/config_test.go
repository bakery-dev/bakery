package config

import (
	"testing"
)

func TestLoadAndGet(t *testing.T) {
	if err := Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	k := Get()
	if k == nil {
		t.Fatal("Get() returned nil koanf instance")
	}
}
