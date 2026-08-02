// Package config loads application configuration using koanf.
package config

import (
	"fmt"
	"os"

	"github.com/adrg/xdg"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/v2"
)

var k *koanf.Koanf

func init() {
	k = koanf.New(".")
}

// Load reads config from XDG config dir, then overlays environment variables.
// File path: $XDG_CONFIG_HOME/bakery/config.yaml
// Env prefix: BAKERY_
func Load() error {
	configPath := fmt.Sprintf("%s/bakery/config.yaml", xdg.ConfigHome)

	// Load YAML file (skip if not found — optional)
	f := file.Provider(configPath)
	if err := k.Load(f, yaml.Parser()); err != nil {
		// File not found is acceptable — config is optional
		if !os.IsNotExist(err) {
			return fmt.Errorf("load config file %s: %w", configPath, err)
		}
	}

	// Overlay env vars with BAKERY_ prefix, split on underscore
	if err := k.Load(env.Provider("BAKERY_", ".", func(s string) string {
		return s
	}), nil); err != nil {
		return fmt.Errorf("load env vars: %w", err)
	}

	return nil
}

// Get returns the global koanf instance.
func Get() *koanf.Koanf {
	return k
}
