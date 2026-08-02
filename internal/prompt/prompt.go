// Package prompt handles interactive TUI prompting using huh.
package prompt

import (
	"fmt"
	"log/slog"

	"github.com/charmbracelet/huh"
	"github.com/bakery-dev/bakery/internal/registry"
	"github.com/bakery-dev/bakery/internal/resolver"
)

// Prompter renders interactive TUI forms for piece prompts.
type Prompter struct {
	log *slog.Logger
}

// New creates a Prompter.
func New(log *slog.Logger) *Prompter {
	return &Prompter{log: log}
}

// Run executes the prompt flow for all pending prompts in the resolution context.
//
// useDefaults: when true, skip interactive prompts and use each prompt's default value.
//
// Returns a map of pieceKey → answer value. Only pieces with prompts are included.
// Pieces that evaluate to "disabled" (false for confirm, empty for select/input) are
// excluded from the enabled set.
func (p *Prompter) Run(ctx *resolver.ResolutionContext, useDefaults bool) (map[string]any, error) {
	if len(ctx.PendingPrompts) == 0 {
		p.log.Debug("no pending prompts")
		return make(map[string]any), nil
	}

	answers := make(map[string]any)

	var fields []huh.Field

	for _, item := range ctx.PendingPrompts {
		field, err := p.buildField(item)
		if err != nil {
			return nil, fmt.Errorf("build prompt for %s: %w", item.PieceKey, err)
		}
		if field != nil {
			fields = append(fields, field)
		}
	}

	if len(fields) == 0 {
		p.log.Debug("all prompts pre-filled, nothing to ask")
		return answers, nil
	}

	group := huh.NewGroup(fields...)
	form := huh.NewForm(group)

	// Non-interactive mode when useDefaults is true.
	if useDefaults {
		form = form.WithAccessible(true)
	}

	if err := form.Run(); err != nil {
		return nil, fmt.Errorf("run prompt form: %w", err)
	}

	// Collect answers from the form.
	for _, item := range ctx.PendingPrompts {
		if isPrefilled(ctx.Pie.Pieces, item.PieceKey) {
			// Already answered in piefile — skip.
			continue
		}

		if !fieldWasBuilt(item) {
			continue
		}

		answer := form.Get(item.PieceKey)
		answers[item.PieceKey] = answer
		p.log.Debug("prompt answered", "piece", item.PieceKey, "answer", answer)
	}

	return answers, nil
}

// buildField creates a huh.Field from a PromptItem.
// Returns nil if the item is pre-filled in the piefile.
func (p *Prompter) buildField(item *resolver.PromptItem) (huh.Field, error) {
	// Already answered in piefile — no field needed.
	if item.Prompt == nil {
		return nil, nil
	}

	switch item.Prompt.Type {
	case "confirm":
		return buildConfirm(item), nil
	case "select":
		return buildSelect(item), nil
	case "multi_select":
		return buildMultiSelect(item), nil
	case "input":
		return buildInput(item), nil
	default:
		return nil, fmt.Errorf("unsupported prompt type %q for piece %s", item.Prompt.Type, item.PieceKey)
	}
}

func buildConfirm(item *resolver.PromptItem) huh.Field {
	c := huh.NewConfirm().
		Key(item.PieceKey).
		Title(item.Prompt.Title)

	if item.Prompt.Default != nil {
		if def, ok := item.Prompt.Default.(bool); ok {
			c = c.Value(&def)
		}
	}

	return c
}

func buildSelect(item *resolver.PromptItem) *huh.Select[string] {
	s := huh.NewSelect[string]().
		Key(item.PieceKey).
		Title(item.Prompt.Title)

	opts := toOptions(item.Prompt.Options)
	if len(opts) > 0 {
		s = s.Options(opts...)
	}

	if item.Prompt.Default != nil {
		if def, ok := item.Prompt.Default.(string); ok {
			// Pre-select matching option.
			for i, o := range opts {
				if o.Key == def || o.Value == def {
					opts[i] = o.Selected(true)
					break
				}
			}
		}
	}

	return s
}

func buildMultiSelect(item *resolver.PromptItem) *huh.MultiSelect[string] {
	m := huh.NewMultiSelect[string]().
		Key(item.PieceKey).
		Title(item.Prompt.Title)

	opts := toOptions(item.Prompt.Options)
	if len(opts) > 0 {
		m = m.Options(opts...)
	}

	return m
}

func buildInput(item *resolver.PromptItem) *huh.Input {
	i := huh.NewInput().
		Key(item.PieceKey).
		Title(item.Prompt.Title)

	if item.Prompt.Default != nil {
		if def, ok := item.Prompt.Default.(string); ok {
			i = i.Placeholder(def).Value(&def)
		}
	}

	return i
}

func toOptions(items []registry.Option) []huh.Option[string] {
	opts := make([]huh.Option[string], 0, len(items))
	for _, item := range items {
		val := fmt.Sprintf("%v", item.Value)
		opts = append(opts, huh.NewOption(item.Label, val))
	}
	return opts
}

// isPrefilled checks if a pie entry has a non-nil answer.
func isPrefilled(entries map[string]*registry.PieEntry, key string) bool {
	entry, ok := entries[key]
	return ok && entry.Answer != nil
}

// fieldWasBuilt checks if a prompt item would have been included in the form
// (i.e., it's not pre-filled).
func fieldWasBuilt(item *resolver.PromptItem) bool {
	return item.Prompt != nil
}
