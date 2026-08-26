package resolver

import (
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/bakery-dev/bakery/internal/registry"
)

// ---------- mock registry ----------

type mockRegistry struct {
	pieces map[string]*registry.Piece
	caps   map[string][]*registry.Piece
}

func newMock() *mockRegistry {
	return &mockRegistry{
		pieces: make(map[string]*registry.Piece),
		caps:   make(map[string][]*registry.Piece),
	}
}

func (m *mockRegistry) Add(key string, p *registry.Piece) {
	m.pieces[key] = p
	for _, cap := range p.Provides {
		m.caps[cap] = append(m.caps[cap], p)
	}
}

func (m *mockRegistry) GetPiece(alias, name string) (*registry.Piece, error) {
	return m.pieces[alias+":"+name], nil
}

func (m *mockRegistry) FindByCapability(capability string) ([]*registry.Piece, error) {
	return m.caps[capability], nil
}

func (m *mockRegistry) AllPieces() map[string]*registry.Piece {
	out := make(map[string]*registry.Piece, len(m.pieces))
	for k, v := range m.pieces {
		out[k] = v
	}
	return out
}

func (m *mockRegistry) PieceDir(alias, name string) string {
	return "/dev/null/pieces/" + alias + "/" + name
}

// ---------- helpers ----------

func newTestLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func boolTrue() *bool { b := true; return &b }
func boolNil() *bool  { return nil }

func pieceNames(pieces []*ResolvedPiece) []string {
	names := make([]string, len(pieces))
	for i, p := range pieces {
		names[i] = p.Piece.Name
	}
	return names
}

// ---------- tests ----------

func TestResolveSimplePie(t *testing.T) {
	reg := newMock()
	reg.Add("core:cobra", &registry.Piece{Name: "cobra", Description: "CLI framework"})
	reg.Add("core:slog", &registry.Piece{Name: "slog", Description: "Structured logging"})

	pie := &registry.Pie{
		Name: "cli",
		Pieces: map[string]*registry.PieEntry{
			"core:cobra": {Answer: boolTrue()},
			"core:slog":  {Answer: boolTrue()},
		},
	}

	ctx, err := New(newTestLog(), reg).Resolve(pie)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.Pieces) != 2 {
		t.Fatalf("expected 2 pieces, got %d", len(ctx.Pieces))
	}
	if len(ctx.PendingPrompts) != 0 {
		t.Errorf("expected 0 pending prompts, got %d", len(ctx.PendingPrompts))
	}
}

func TestResolveWithDependency(t *testing.T) {
	reg := newMock()
	reg.Add("core:vite", &registry.Piece{Name: "vite", Description: "Vite"})
	reg.Add("frontend:react", &registry.Piece{
		Name: "react", Description: "React", DependsOn: []string{"core:vite"},
	})

	pie := &registry.Pie{
		Name: "web",
		Pieces: map[string]*registry.PieEntry{
			"frontend:react": {Answer: boolTrue()},
		},
	}

	ctx, err := New(newTestLog(), reg).Resolve(pie)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.Pieces) != 2 {
		t.Fatalf("expected 2 pieces, got %d: %v", len(ctx.Pieces), pieceNames(ctx.Pieces))
	}
	// Topo order: vite before react.
	if ctx.Pieces[0].Piece.Name != "vite" {
		t.Errorf("first piece = %s, want vite", ctx.Pieces[0].Piece.Name)
	}
	if ctx.Pieces[1].Piece.Name != "react" {
		t.Errorf("second piece = %s, want react", ctx.Pieces[1].Piece.Name)
	}
}

func TestResolveWithCapability(t *testing.T) {
	reg := newMock()
	reg.Add("core:vite", &registry.Piece{Name: "vite"})
	reg.Add("frontend:react", &registry.Piece{
		Name: "react", Provides: []string{"frontend-framework"}, DependsOn: []string{"core:vite"},
	})
	reg.Add("fullstack:remix", &registry.Piece{
		Name: "remix", DependsOn: []string{"frontend-framework"},
	})

	pie := &registry.Pie{
		Name: "fullstack",
		Pieces: map[string]*registry.PieEntry{
			"frontend:react":  {Answer: boolTrue()},
			"fullstack:remix": {Answer: boolTrue()},
		},
	}

	ctx, err := New(newTestLog(), reg).Resolve(pie)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.Pieces) != 3 {
		t.Fatalf("expected 3 pieces, got %d: %v", len(ctx.Pieces), pieceNames(ctx.Pieces))
	}
}

func TestResolveCapabilityCollision(t *testing.T) {
	reg := newMock()
	reg.Add("frontend:react", &registry.Piece{
		Name: "react", Provides: []string{"frontend-framework"},
	})
	reg.Add("frontend:angular", &registry.Piece{
		Name: "angular", Provides: []string{"frontend-framework"},
	})

	pie := &registry.Pie{
		Name: "collision",
		Pieces: map[string]*registry.PieEntry{
			"frontend:react":   {Answer: boolTrue()},
			"frontend:angular": {Answer: boolTrue()},
		},
	}

	_, err := New(newTestLog(), reg).Resolve(pie)
	if err == nil {
		t.Fatal("expected capability collision error, got nil")
	}
	if !strings.Contains(err.Error(), "capability collision") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestResolveCircularDependency(t *testing.T) {
	reg := newMock()
	reg.Add("core:a", &registry.Piece{Name: "a", DependsOn: []string{"core:b"}})
	reg.Add("core:b", &registry.Piece{Name: "b", DependsOn: []string{"core:a"}})

	pie := &registry.Pie{
		Name: "circular",
		Pieces: map[string]*registry.PieEntry{
			"core:a": {Answer: boolTrue()},
		},
	}

	_, err := New(newTestLog(), reg).Resolve(pie)
	if err == nil {
		t.Fatal("expected circular dependency error, got nil")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("wrong error: %v", err)
	}
}

func TestResolveMissingPiece(t *testing.T) {
	reg := newMock()
	pie := &registry.Pie{
		Name: "missing",
		Pieces: map[string]*registry.PieEntry{
			"unknown:ghost": {Answer: boolTrue()},
		},
	}

	_, err := New(newTestLog(), reg).Resolve(pie)
	if err == nil {
		t.Fatal("expected missing piece error, got nil")
	}
}

func TestResolvePendingPrompts(t *testing.T) {
	reg := newMock()
	reg.Add("core:cobra", &registry.Piece{
		Name:   "cobra",
		Prompt: &registry.Prompt{Type: "confirm", Title: "Include Cobra?"},
	})

	pie := &registry.Pie{
		Name: "prompt-test",
		Pieces: map[string]*registry.PieEntry{
			"core:cobra": {Answer: boolNil()},
		},
	}

	ctx, err := New(newTestLog(), reg).Resolve(pie)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.PendingPrompts) != 1 {
		t.Fatalf("expected 1 pending prompt, got %d", len(ctx.PendingPrompts))
	}
	if ctx.PendingPrompts[0].PieceKey != "core:cobra" {
		t.Errorf("prompt pieceKey = %q, want core:cobra", ctx.PendingPrompts[0].PieceKey)
	}
}

func TestResolvePreFilledSkipsPrompt(t *testing.T) {
	reg := newMock()
	reg.Add("core:cobra", &registry.Piece{
		Name:   "cobra",
		Prompt: &registry.Prompt{Type: "confirm", Title: "Include Cobra?"},
	})

	pie := &registry.Pie{
		Name: "prefill-test",
		Pieces: map[string]*registry.PieEntry{
			"core:cobra": {Answer: boolTrue()},
		},
	}

	ctx, err := New(newTestLog(), reg).Resolve(pie)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.PendingPrompts) != 0 {
		t.Errorf("expected 0 pending prompts (pre-filled), got %d", len(ctx.PendingPrompts))
	}
}

func TestResolveTransitiveDependency(t *testing.T) {
	reg := newMock()
	reg.Add("core:base", &registry.Piece{Name: "base"})
	reg.Add("core:mid", &registry.Piece{Name: "mid", DependsOn: []string{"core:base"}})
	reg.Add("app:top", &registry.Piece{Name: "top", DependsOn: []string{"core:mid"}})

	pie := &registry.Pie{
		Name: "chain",
		Pieces: map[string]*registry.PieEntry{
			"app:top": {Answer: boolTrue()},
		},
	}

	ctx, err := New(newTestLog(), reg).Resolve(pie)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.Pieces) != 3 {
		t.Fatalf("expected 3 pieces (transitive), got %d: %v", len(ctx.Pieces), pieceNames(ctx.Pieces))
	}
	// Order: base → mid → top
	if ctx.Pieces[0].Piece.Name != "base" {
		t.Errorf("first = %s, want base", ctx.Pieces[0].Piece.Name)
	}
	if ctx.Pieces[2].Piece.Name != "top" {
		t.Errorf("last = %s, want top", ctx.Pieces[2].Piece.Name)
	}
}
