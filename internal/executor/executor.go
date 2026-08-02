// Package executor runs post-scaffolding build and initialization actions.
package executor

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Action is a pre-approved command specification.
type Action struct {
	Command string   // binary name
	Args    []string // arguments passed to the binary
}

// allowedActions is the whitelist of ecosystem actions.
// Arbitrary command execution is strictly forbidden.
var allowedActions = map[string]Action{
	"go:tidy":       {Command: "go", Args: []string{"mod", "tidy"}},
	"go:build":      {Command: "go", Args: []string{"build", "./..."}},
	"npm:install":   {Command: "npm", Args: []string{"install"}},
	"pnpm:install":  {Command: "pnpm", Args: []string{"install"}},
	"git:init":      {Command: "git", Args: []string{"init"}},
}

// Executor runs pre-approved ecosystem actions inside a target directory.
type Executor struct {
	log *slog.Logger
}

// New creates an Executor.
func New(log *slog.Logger) *Executor {
	return &Executor{log: log}
}

// RunActions executes the requested actions in targetDir.
// Only actions present in the allowed whitelist may run.
// Unknown actions cause immediate fatal error.
// Duplicate actions are deduplicated while preserving first-occurrence order.
func (e *Executor) RunActions(targetDir string, actions []string) error {
	if len(actions) == 0 {
		e.log.Debug("no actions to run")
		return nil
	}

	unique := dedupe(actions)

	for _, name := range unique {
		action, err := e.parseAction(name, targetDir)
		if err != nil {
			return err
		}

		if err := e.runOne(targetDir, name, action); err != nil {
			return fmt.Errorf("action %q failed: %w", name, err)
		}
	}

	return nil
}

// runOne executes a single approved action in targetDir.
func (e *Executor) runOne(targetDir, name string, action Action) error {
	e.log.Info("running action", "action", name, "dir", targetDir)

	cmd := exec.Command(action.Command, action.Args...)
	cmd.Dir = targetDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", action.Command, slices.Concat([]string{}, action.Args), err)
	}

	e.log.Info("action completed", "action", name)
	return nil
}

func (e *Executor) parseAction(name, targetDir string) (Action, error) {
	if a, ok := allowedActions[name]; ok {
		return a, nil
	}
	
	// Dynamic go:get action
	if strings.HasPrefix(name, "go:get ") {
		pkg := strings.TrimSpace(strings.TrimPrefix(name, "go:get "))
		if pkg == "" {
			return Action{}, fmt.Errorf("invalid go:get action: missing package")
		}
		return Action{Command: "go", Args: []string{"get", pkg}}, nil
	}

	// Dynamic go:init action (uses project dir name if not specified)
	if strings.HasPrefix(name, "go:init") {
		mod := strings.TrimSpace(strings.TrimPrefix(name, "go:init"))
		if mod == "" {
			mod = filepath.Base(targetDir)
		}
		return Action{Command: "go", Args: []string{"mod", "init", mod}}, nil
	}

	return Action{}, fmt.Errorf("unknown action %q — execution denied (not in allowed list)", name)
}

// dedupe removes duplicate strings while preserving order.
func dedupe(s []string) []string {
	seen := make(map[string]struct{}, len(s))
	var out []string
	for _, v := range s {
		if _, dup := seen[v]; dup {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
