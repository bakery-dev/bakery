package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/bakery-dev/bakery/internal/logger"
)

var updateCmd = &cobra.Command{
	Use:   "update [alias]",
	Short: "Update one or all registered registries",
	Long: `Fetch the latest commits and checkout for registered registries.

  bakery update          # update all
  bakery update core     # update only 'core'`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runUpdate(args)
	},
}

func runUpdate(args []string) error {
	l := logger.WithModule("update")

	reg, err := buildRegistry()
	if err != nil {
		return fmt.Errorf("build registry: %w", err)
	}

	if len(args) > 0 {
		// Update single alias.
		alias := args[0]
		url, err := getRegistryURL(alias)
		if err != nil {
			return fmt.Errorf("lookup alias %q: %w", alias, err)
		}
		if err := reg.Update(alias, url); err != nil {
			return fmt.Errorf("update %s: %w", alias, err)
		}
		l.Info("registry updated", "alias", alias)
		fmt.Printf("Updated registry: %s\n", alias)
		return nil
	}

	// Update all registered registries.
	cfg, err := loadConfigMap()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	regMap, ok := cfg["registries"].(map[string]any)
	if !ok {
		l.Info("no registries configured")
		fmt.Println("No registries configured. Use 'bakery repo add' first.")
		return nil
	}

	var failed []string
	for alias, urlAny := range regMap {
		url, ok := urlAny.(string)
		if !ok {
			failed = append(failed, alias)
			continue
		}
		if err := reg.Update(alias, url); err != nil {
			l.Warn("update failed", "alias", alias, "error", err)
			failed = append(failed, alias)
			continue
		}
		l.Info("registry updated", "alias", alias)
	}

	if len(failed) > 0 {
		return fmt.Errorf("failed to update %d registry(ies): %v", len(failed), failed)
	}

	fmt.Println("All registries updated.")
	return nil
}

// getRegistryURL looks up a registry alias URL from config.
func getRegistryURL(alias string) (string, error) {
	cfg, err := loadConfigMap()
	if err != nil {
		return "", err
	}

	regMap, ok := cfg["registries"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("no registries in config")
	}

	url, ok := regMap[alias]
	if !ok {
		return "", fmt.Errorf("alias %q not found in config", alias)
	}

	if u, ok := url.(string); ok {
		return u, nil
	}
	return "", fmt.Errorf("alias %q has invalid URL type", alias)
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
