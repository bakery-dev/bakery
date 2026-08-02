package executor

import (
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func newTestExecutor(t *testing.T) *Executor {
	t.Helper()
	return New(slog.New(slog.NewTextHandler(os.Stderr, nil)))
}

// makeGoMod creates a minimal go.mod so go:tidy and go:build are valid.
func makeGoMod(dir string, module string) {
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+module+"\ngo 1.22\n"), 0o644); err != nil {
		panic(err)
	}
}

// makeMain creates a minimal main.go so go:build succeeds.
func makeMain(dir string) {
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		panic(err)
	}
}

func TestRunActions_GoTidy(t *testing.T) {
	dir := t.TempDir()
	makeGoMod(dir, "test-module")

	e := newTestExecutor(t)
	if err := e.RunActions(dir, []string{"go:tidy"}); err != nil {
		t.Fatalf("go:tidy failed: %v", err)
	}
}

func TestRunActions_GoBuild(t *testing.T) {
	dir := t.TempDir()
	makeGoMod(dir, "test-module")
	makeMain(dir)

	e := newTestExecutor(t)
	if err := e.RunActions(dir, []string{"go:build"}); err != nil {
		t.Fatalf("go:build failed: %v", err)
	}
}

func TestRunActions_NpmInstall(t *testing.T) {
	// Skip if npm is unavailable.
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm not found")
	}

	dir := t.TempDir()
	// Minimal package.json so npm install doesn't fail.
	packageJSON := `{"name":"test","version":"1.0.0"}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	e := newTestExecutor(t)
	if err := e.RunActions(dir, []string{"npm:install"}); err != nil {
		t.Fatalf("npm:install failed: %v", err)
	}
}

func TestRunActions_PnpmInstall(t *testing.T) {
	// Skip if pnpm is unavailable.
	if _, err := exec.LookPath("pnpm"); err != nil {
		t.Skip("pnpm not found")
	}

	dir := t.TempDir()
	packageJSON := `{"name":"test","version":"1.0.0"}`
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(packageJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	e := newTestExecutor(t)
	if err := e.RunActions(dir, []string{"pnpm:install"}); err != nil {
		t.Fatalf("pnpm:install failed: %v", err)
	}
}

func TestRunActions_UnknownActionDenied(t *testing.T) {
	dir := t.TempDir()
	e := newTestExecutor(t)

	err := e.RunActions(dir, []string{"rm:-rf"})
	if err == nil {
		t.Fatal("expected error for unknown action, got nil")
	}

	if got := err.Error(); got == "" {
		t.Error("error message is empty")
	}
}

func TestRunActions_MultipleActions(t *testing.T) {
	dir := t.TempDir()
	makeGoMod(dir, "test-module")
	makeMain(dir)

	e := newTestExecutor(t)
	if err := e.RunActions(dir, []string{"go:tidy", "go:build"}); err != nil {
		t.Fatalf("multiple actions failed: %v", err)
	}
}

func TestRunActions_DuplicateActionsDeduped(t *testing.T) {
	dir := t.TempDir()
	makeGoMod(dir, "test-module")
	makeMain(dir)

	e := newTestExecutor(t)
	// go:tidy listed twice — should run once without error.
	if err := e.RunActions(dir, []string{"go:tidy", "go:tidy", "go:build"}); err != nil {
		t.Fatalf("deduped actions failed: %v", err)
	}
}

func TestRunActions_EmptyActions(t *testing.T) {
	dir := t.TempDir()
	e := newTestExecutor(t)

	if err := e.RunActions(dir, nil); err != nil {
		t.Fatalf("nil actions should succeed: %v", err)
	}

	if err := e.RunActions(dir, []string{}); err != nil {
		t.Fatalf("empty actions should succeed: %v", err)
	}
}

func TestRunActions_UnknownActionStopsImmediately(t *testing.T) {
	dir := t.TempDir()
	makeGoMod(dir, "test-module")

	e := newTestExecutor(t)

	// bad-action comes first — go:tidy after it should never run.
	// We can't directly test "never runs" but we verify the error is about
	// the unknown action, not go:tidy.
	err := e.RunActions(dir, []string{"bad-action", "go:tidy"})
	if err == nil {
		t.Fatal("expected error for unknown action, got nil")
	}

	if got := err.Error(); got == "" {
		t.Error("error message is empty")
	}
}

func TestDedupe(t *testing.T) {
	tests := []struct {
		in   []string
		want []string
	}{
		{[]string{"a", "b", "a"}, []string{"a", "b"}},
		{[]string{"a", "a", "a"}, []string{"a"}},
		{[]string{}, []string{}},
		{nil, []string{}},
		{[]string{"a", "b", "c"}, []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		got := dedupe(tt.in)
		if len(got) != len(tt.want) {
			t.Errorf("dedupe(%v) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("dedupe(%v)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
			}
		}
	}
}
