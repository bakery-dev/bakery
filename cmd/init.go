package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bakery-dev/bakery/internal/config"
	"github.com/bakery-dev/bakery/internal/engine"
	"github.com/bakery-dev/bakery/internal/executor"
	"github.com/bakery-dev/bakery/internal/logger"
	"github.com/bakery-dev/bakery/internal/prompt"
	"github.com/bakery-dev/bakery/internal/registry"
	"github.com/bakery-dev/bakery/internal/resolver"
)

var initCmd = &cobra.Command{
	Use:   "init <pie> <project>",
	Short: "Scaffold a new project from a Pie",
	Long: `Resolve a Pie, prompt for piece options, render templates, and run actions.

Example:
  bakery init core:cli my-app
  bakery init core:cli my-app --defaults`,
	Args:         cobra.ExactArgs(2),
	SilenceUsage: true,
	RunE: func(_ *cobra.Command, args []string) error {
		return runInit(args[0], args[1])
	},
}

// registryOverrides holds parsed --registry flags (alias=path or alias=url).
var registryOverrides []string

func runInit(pieRef, projectDir string) error {
	l := logger.WithModule("init")

	// Parse pie reference.
	pieName, err := extractPieName(pieRef)
	if err != nil {
		return fmt.Errorf("invalid pie ref %q: %w", pieRef, err)
	}

	// Build registry from config, then apply --registry overrides.
	overrides, err := parseRegistryFlags(registryOverrides)
	if err != nil {
		return fmt.Errorf("parse --registry: %w", err)
	}
	reg, err := buildRegistry(overrides)
	if err != nil {
		return fmt.Errorf("build registry: %w", err)
	}

	// Update all registries.
	if err := updateAllRegistries(reg, l); err != nil {
		return fmt.Errorf("update registries: %w", err)
	}

	// Load the pie.
	pie, err := reg.FindPie(pieName)
	if err != nil {
		return fmt.Errorf("find pie %q: %w", pieName, err)
	}
	l.Info("loaded pie", "name", pie.Name)

	// Resolve pieces.
	res := resolver.New(l, reg)
	ctx, err := res.Resolve(pie)
	if err != nil {
		return fmt.Errorf("resolve pie: %w", err)
	}
	l.Info("resolved pieces", "count", len(ctx.Pieces))

	// Run prompts.
	p := prompt.New(l)
	answers, err := p.Run(ctx, useDefaults)
	if err != nil {
		return fmt.Errorf("run prompts: %w", err)
	}
	if answers == nil {
		answers = make(map[string]any)
	}
	answers["ProjectName"] = filepath.Base(projectDir)

	// Determine active pieces and collect actions.
	activePieces, actions := collectActivePieces(ctx, answers)

	// Collect commit hashes.
	commits := collectCommits(reg, activePieces)

	// Execute templates.
	eng := engine.New(l)
	if err := eng.Execute(projectDir, activePieces, answers, commits); err != nil {
		return fmt.Errorf("execute templates: %w", err)
	}
	l.Info("templates rendered", "dir", projectDir)

	// Run actions.
	ex := executor.New(l)
	if err := ex.RunActions(projectDir, actions); err != nil {
		return fmt.Errorf("run actions: %w", err)
	}

	l.Info("project scaffolded", "dir", projectDir)
	return nil
}

// buildRegistry creates a Registry and populates it from the koanf config.
// Config keys: "registries.alias" = "<url>"
// overrides (from --registry flags) are applied last and take precedence.
func buildRegistry(overrides map[string]string) (*registry.Registry, error) {
	l := logger.WithModule("registry")
	reg := registry.New(l)

	ko := config.Get()
	// Read "registries.<alias>" entries.
	// koanf uses dot-separated keys.
	for _, key := range ko.Keys() {
		parts := strings.SplitN(key, ".", 2)
		if len(parts) != 2 || parts[0] != "registries" {
			continue
		}
		alias := parts[1]
		url := ko.String(key)
		if url == "" {
			continue
		}
		if _, overridden := overrides[alias]; overridden {
			continue // --registry override will handle this alias.
		}
		if err := reg.Update(alias, url); err != nil {
			l.Warn("update registry failed (will retry later)", "alias", alias, "error", err)
		}
	}

	// Apply --registry overrides last so they win over config entries.
	for alias, val := range overrides {
		if err := reg.Update(alias, resolveRegistryValue(val)); err != nil {
			return nil, fmt.Errorf("override registry %q: %w", alias, err)
		}
		l.Info("registry overridden", "alias", alias, "value", val)
	}

	return reg, nil
}

// parseRegistryFlags parses repeated --registry flags of form "alias=value".
// value may be a local directory path or a remote repo URL.
func parseRegistryFlags(flags []string) (map[string]string, error) {
	out := make(map[string]string, len(flags))
	for _, f := range flags {
		idx := strings.Index(f, "=")
		if idx <= 0 || idx == len(f)-1 {
			return nil, fmt.Errorf("expected alias=path, got %q", f)
		}
		out[f[:idx]] = f[idx+1:]
	}
	return out, nil
}

// resolveRegistryValue normalizes a --registry flag value for the registry.
// An existing directory is resolved to an absolute path so it is treated as a
// local registry; anything else is passed through as a remote repo URL.
func resolveRegistryValue(val string) string {
	if info, err := os.Stat(val); err == nil && info.IsDir() {
		if abs, err := filepath.Abs(val); err == nil {
			return abs
		}
	}
	return val
}

// updateAllRegistries calls Update for every registered registry alias.
func updateAllRegistries(_ *registry.Registry, _ *slog.Logger) error {
	// Registries are already updated in buildRegistry via config.
	// This is a no-op for now; kept for explicit update hooks.
	return nil
}

// extractPieName returns the pie name from "alias:name" or plain "name".
// Splits on the LAST ':' so URLs like "file:///path/to/repo:golang" work.
func extractPieName(ref string) (string, error) {
	idx := strings.LastIndex(ref, ":")
	if idx == -1 {
		return ref, nil
	}
	return ref[idx+1:], nil
}

// collectActivePieces filters resolved pieces to those enabled by prompt answers.
// For "confirm" prompts, answer must be true.
// For other prompts, answer must be non-empty.
func collectActivePieces(ctx *resolver.ResolutionContext, answers map[string]any) ([]*resolver.ResolvedPiece, []string) {
	var active []*resolver.ResolvedPiece
	var actions []string

	for _, piece := range ctx.Pieces {
		if !isActive(piece, answers) {
			continue
		}
		active = append(active, piece)
		actions = append(actions, piece.Piece.Actions...)
	}

	return active, actions
}

// isActive checks if a piece is enabled based on its prompt answer.
func isActive(piece *resolver.ResolvedPiece, answers map[string]any) bool {
	if piece.Piece.Prompt == nil {
		return true
	}

	answer, ok := answers[piece.Key]
	if !ok {
		return true
	}

	switch piece.Piece.Prompt.Type {
	case "confirm":
		if b, ok := answer.(bool); ok {
			return b
		}
		return false
	default:
		switch v := answer.(type) {
		case string:
			return v != ""
		case []string:
			return len(v) > 0
		default:
			return true
		}
	}
}

// collectCommits builds pieceKey -> commit hash map for active pieces.
func collectCommits(reg *registry.Registry, pieces []*resolver.ResolvedPiece) map[string]string {
	commits := make(map[string]string, len(pieces))
	for _, piece := range pieces {
		alias, name, ok := parsePieceKey(piece.Key)
		if !ok {
			commits[piece.Key] = "unknown"
			continue
		}
		commit := reg.GetCommit(alias)
		if commit == "" {
			commit = "resolved"
		}
		commits[piece.Key] = commit
		_ = name
	}
	return commits
}

// parsePieceKey splits "alias:name" into parts.
// Splits on the LAST ':' so URLs with colons in the alias (e.g. file://) work.
func parsePieceKey(key string) (alias, name string, ok bool) {
	idx := strings.LastIndex(key, ":")
	if idx == -1 {
		return "", "", false
	}
	return key[:idx], key[idx+1:], true
}

func init() {
	initCmd.Flags().StringArrayVar(&registryOverrides, "registry", nil,
		"Override a registry alias with a local path or a different repo URL (alias=path). Repeatable.")
	rootCmd.AddCommand(initCmd)
}
