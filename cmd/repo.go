package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/bakery-dev/bakery/internal/config"
)

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Manage piece registries",
	Long:  `Add or remove piece registries. Registries are stored in the config file.`,
}

var repoAddCmd = &cobra.Command{
	Use:   "add <alias> <url>",
	Short: "Register a new piece registry",
	Long: `Add a piece registry alias mapped to a git URL or local path.

Examples:
  bakery repo add core github.com/bakery-dev/bakery
  bakery repo add local file:///path/to/local-registry`,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runRepoAdd(args[0], args[1])
	},
}

var repoRemoveCmd = &cobra.Command{
	Use:   "remove <alias>",
	Short: "Remove a registered piece registry",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return runRepoRemove(args[0])
	},
}

func runRepoAdd(alias, url string) error {
	cfg, err := loadConfigMap()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	regMap, ok := cfg["registries"].(map[string]any)
	if !ok {
		regMap = make(map[string]any)
	}

	regMap[alias] = url
	cfg["registries"] = regMap

	if err := saveConfigMap(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Registry added: %s -> %s\n", alias, url)
	return nil
}

func runRepoRemove(alias string) error {
	cfg, err := loadConfigMap()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	regMap, ok := cfg["registries"].(map[string]any)
	if !ok {
		return fmt.Errorf("no registries in config")
	}

	if _, exists := regMap[alias]; !exists {
		return fmt.Errorf("registry %q not found", alias)
	}

	delete(regMap, alias)
	cfg["registries"] = regMap

	if err := saveConfigMap(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	fmt.Printf("Registry removed: %s\n", alias)
	return nil
}

// loadConfigMap reads the config YAML into a map.
func loadConfigMap() (map[string]any, error) {
	path := configPath()
	cfg := make(map[string]any)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return cfg, nil
}

// saveConfigMap writes the config map back as YAML.
func saveConfigMap(cfg map[string]any) error {
	path := configPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}

	// Also reload into koanf for runtime use.
	if err := config.Load(); err != nil {
		return fmt.Errorf("reload config: %w", err)
	}

	return nil
}

// configPath returns the XDG config path for bakery.
func configPath() string {
	return fmt.Sprintf("%s/bakery/config.yaml", xdg.ConfigHome)
}

func init() {
	repoCmd.AddCommand(repoAddCmd)
	repoCmd.AddCommand(repoRemoveCmd)
	rootCmd.AddCommand(repoCmd)
}
