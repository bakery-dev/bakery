// Package cmd defines the command-line interface commands using Cobra.
package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/bakery-dev/bakery/internal/config"
	"github.com/bakery-dev/bakery/internal/logger"
)

// Global flags.
var (
	logLevel    string
	useDefaults bool
	pieFile     string
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "bakery",
	Short: "Scaffolding CLI powered by composable pieces",
	Long: `Bakery assembles project scaffolds from composable pieces.

Run 'bakery init <pie> <project>' to scaffold a new project.
Run 'bakery repo add <alias> <url>' to register a piece registry.`,
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		logger.Setup(logLevel)
		l := logger.WithModule("root")

		if err := config.Load(); err != nil {
			l.Warn("loading config", "error", err)
		} else {
			l.Info("config loaded")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Help()
	},
}

// Execute adds child commands and runs the CLI.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "INFO",
		"Log level: DEBUG, INFO, WARN, ERROR")
	rootCmd.PersistentFlags().BoolVar(&useDefaults, "defaults", false,
		"Skip interactive prompts and use default values")
	rootCmd.PersistentFlags().StringVar(&pieFile, "piefile", "",
		"Path to an existing piefile.yaml for non-interactive scaffolding")
}
