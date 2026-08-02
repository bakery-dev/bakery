package prompt

import (
	"log/slog"
	"os"
	"reflect"
	"testing"

	"github.com/bakery-dev/bakery/internal/registry"
	"github.com/bakery-dev/bakery/internal/resolver"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// promptEntry builds a ResolutionContext with the given pending prompts.
func ctxWith(prompts []*resolver.PromptItem) *resolver.ResolutionContext {
	return &resolver.ResolutionContext{Pie: &registry.Pie{}, PendingPrompts: prompts}
}

func TestRunDefaultsUsesConfiguredDefault(t *testing.T) {
	ctx := ctxWith([]*resolver.PromptItem{{
		PieceKey: "bakery-dev/bakery:git",
		Piece:    &registry.Piece{},
		Prompt:   &registry.Prompt{Type: "confirm", Title: "Init git?", Default: true},
	}})

	got, err := New(newTestLogger()).Run(ctx, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := map[string]any{"bakery-dev/bakery:git": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("answers = %v, want %v", got, want)
	}
}

func TestRunDefaultsFallsBackToZeroValue(t *testing.T) {
	ctx := ctxWith([]*resolver.PromptItem{
		{PieceKey: "a:confirm", Piece: &registry.Piece{}, Prompt: &registry.Prompt{Type: "confirm", Title: "X"}},
		{PieceKey: "a:input", Piece: &registry.Piece{}, Prompt: &registry.Prompt{Type: "input", Title: "Y"}},
	})

	got, err := New(newTestLogger()).Run(ctx, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got["a:confirm"] != false {
		t.Errorf("confirm default = %v, want false", got["a:confirm"])
	}
	if got["a:input"] != "" {
		t.Errorf("input default = %q, want empty", got["a:input"])
	}
}

func TestRunDefaultsSkipsPrefilled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	pie := &registry.Pie{
		Pieces: map[string]*registry.PieEntry{
			"core:thing": {Answer: boolPtr(false)},
		},
	}
	ctx := &resolver.ResolutionContext{Pie: pie, PendingPrompts: []*resolver.PromptItem{{
		PieceKey: "core:thing",
		Piece:    &registry.Piece{},
		Prompt:   &registry.Prompt{Type: "confirm", Title: "Thing?", Default: true},
	}}}

	got, err := New(newTestLogger()).Run(ctx, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, present := got["core:thing"]; present {
		t.Errorf("pre-filled piece should be skipped, got %v", got["core:thing"])
	}
}

func boolPtr(b bool) *bool { return &b }
