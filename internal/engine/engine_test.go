package engine

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bakery-dev/bakery/internal/resolver"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return New(log)
}

func newTestPiece(dir string) *resolver.ResolvedPiece {
	return &resolver.ResolvedPiece{
		Key: "core:test-piece",
		Dir: dir,
	}
}

func TestExecute_RendersTemplate(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	// Create template directory with a file.
	tmplDir := filepath.Join(srcDir, "template")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}

	tmplContent := `Project: {{ index . "core:test-piece" }}`
	if err := os.WriteFile(filepath.Join(tmplDir, "README.md"), []byte(tmplContent), 0o644); err != nil {
		t.Fatal(err)
	}

	answers := map[string]any{
		"core:test-piece": "My Project",
	}
	commits := map[string]string{
		"core:test-piece": "abc123",
	}

	pieces := []*resolver.ResolvedPiece{newTestPiece(srcDir)}

	e := newTestEngine(t)
	if err := e.Execute(targetDir, pieces, answers, commits); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify README.md rendered.
	got, err := os.ReadFile(filepath.Join(targetDir, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	want := "Project: My Project"
	if string(got) != want {
		t.Errorf("README.md = %q, want %q", got, want)
	}

	// Verify piefile.yaml written.
	if _, err := os.Stat(filepath.Join(targetDir, "piefile.yaml")); err != nil {
		t.Errorf("piefile.yaml not found: %v", err)
	}

	// Verify pielock written.
	if _, err := os.Stat(filepath.Join(targetDir, "pielock")); err != nil {
		t.Errorf("pielock not found: %v", err)
	}
}

func TestExecute_PathTraversalSanitized(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	tmplDir := filepath.Join(srcDir, "template")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a normal template file — verify filepath.Clean prevents any escape.
	if err := os.WriteFile(filepath.Join(tmplDir, "safe.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	pieces := []*resolver.ResolvedPiece{newTestPiece(srcDir)}
	e := newTestEngine(t)

	if err := e.Execute(targetDir, pieces, nil, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Verify file landed inside targetDir (not escaped).
	dest := filepath.Join(targetDir, "safe.txt")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("safe.txt not found at expected path %s: %v", dest, err)
	}

	// Verify no files outside targetDir.
	targetParent := filepath.Dir(targetDir)
	entries, _ := os.ReadDir(targetParent)
	for _, entry := range entries {
		if entry.Name() == "safe.txt" {
			t.Error("path traversal: safe.txt landed outside targetDir")
		}
	}
}

func TestExecute_SymlinkProhibited(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	tmplDir := filepath.Join(srcDir, "template")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a real file and a symlink to it.
	realFile := filepath.Join(srcDir, "real.txt")
	if err := os.WriteFile(realFile, []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(tmplDir, "link.txt")
	if err := os.Symlink(realFile, symlinkPath); err != nil {
		t.Skip("cannot create symlinks")
	}

	pieces := []*resolver.ResolvedPiece{newTestPiece(srcDir)}
	e := newTestEngine(t)

	err := e.Execute(targetDir, pieces, nil, nil)
	if err == nil {
		t.Error("expected symlink error, got nil")
	}
}

func TestExecute_OverwriteWarns(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	tmplDir := filepath.Join(srcDir, "template")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(tmplDir, "file.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pre-create the target file.
	if err := os.WriteFile(filepath.Join(targetDir, "file.txt"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	pieces := []*resolver.ResolvedPiece{newTestPiece(srcDir)}
	e := newTestEngine(t)

	if err := e.Execute(targetDir, pieces, nil, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// File should be overwritten.
	got, err := os.ReadFile(filepath.Join(targetDir, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("file.txt = %q, want \"new\"", got)
	}
}

func TestExecute_NestedTemplates(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	tmplDir := filepath.Join(srcDir, "template")
	nestedDir := filepath.Join(tmplDir, "config", "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(nestedDir, "app.yaml"), []byte("key: val"), 0o644); err != nil {
		t.Fatal(err)
	}

	pieces := []*resolver.ResolvedPiece{newTestPiece(srcDir)}
	e := newTestEngine(t)

	if err := e.Execute(targetDir, pieces, nil, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	dest := filepath.Join(targetDir, "config", "nested", "app.yaml")
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("nested file not found at %s: %v", dest, err)
	}
}

func TestExecute_NoTemplateDir(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	// Piece dir exists but no template/ subdirectory.
	pieces := []*resolver.ResolvedPiece{newTestPiece(srcDir)}
	e := newTestEngine(t)

	if err := e.Execute(targetDir, pieces, nil, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Should succeed — piefile.yaml and pielock still written.
	if _, err := os.Stat(filepath.Join(targetDir, "piefile.yaml")); err != nil {
		t.Error("piefile.yaml not found")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "pielock")); err != nil {
		t.Error("pielock not found")
	}
}

func TestExecute_NoDir(t *testing.T) {
	srcDir := t.TempDir()
	pieces := []*resolver.ResolvedPiece{
		{Key: "core:empty", Dir: ""},
	}
	e := newTestEngine(t)

	if err := e.Execute(srcDir, pieces, nil, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestExecute_PielockContent(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	tmplDir := filepath.Join(srcDir, "template")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "dummy.txt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	commits := map[string]string{
		"core:foo": "deadbeef",
		"core:bar": "cafe4242",
	}

	pieces := []*resolver.ResolvedPiece{newTestPiece(srcDir)}
	e := newTestEngine(t)

	if err := e.Execute(targetDir, pieces, nil, commits); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(targetDir, "pielock"))
	if err != nil {
		t.Fatal(err)
	}

	content := string(got)
	// Must use "resolved" key (not "pieces").
	if !contains(content, "resolved") {
		t.Errorf("pielock missing 'resolved' key: %s", content)
	}
	if !contains(content, "deadbeef") {
		t.Errorf("pielock missing commit deadbeef: %s", content)
	}
	if !contains(content, "cafe4242") {
		t.Errorf("pielock missing commit cafe4242: %s", content)
	}
}

func TestExecute_PiefileContent(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	tmplDir := filepath.Join(srcDir, "template")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "dummy.txt"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	answers := map[string]any{
		"core:foo": true,
		"core:bar": "selected-value",
	}

	pieces := []*resolver.ResolvedPiece{
		{Key: "core:foo", Dir: srcDir},
		{Key: "core:bar", Dir: srcDir},
	}
	e := newTestEngine(t)

	if err := e.Execute(targetDir, pieces, answers, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(targetDir, "piefile.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	content := string(got)
	if !contains(content, "core:foo") {
		t.Errorf("piefile missing key core:foo: %s", content)
	}
	if !contains(content, "selected-value") {
		t.Errorf("piefile missing answer selected-value: %s", content)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// writeTemplateFile creates a file under a piece's template/ directory at the
// given relative (potentially token-bearing) path.
func writeTemplateFile(t *testing.T, tmplDir, relPath, content string) {
	t.Helper()
	full := filepath.Join(tmplDir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRenderSegment(t *testing.T) {
	wk := map[string]string{"ProjectName": "myapp"}

	tests := []struct {
		name    string
		seg     string
		want    string
		wantErr string
	}{
		{name: "known token", seg: "__ProjectName__", want: "myapp"},
		{name: "token plus extension", seg: "__ProjectName__.go", want: "myapp.go"},
		{name: "token embedded in name", seg: "cmd-__ProjectName__", want: "cmd-myapp"},
		{name: "literal passthrough", seg: "root.go", want: "root.go"},
		{name: "unknown token errors", seg: "__Foo__", wantErr: "unknown path variable __Foo__"},
		{name: "typo token errors", seg: "__ProjecName__", wantErr: "unknown path variable __ProjecName__"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := renderSegment(tc.seg, wk)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (result=%q)", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderSegment_RejectsUnsafeValues(t *testing.T) {
	cases := map[string]string{
		"value with slash": "a/b",
		"value traversal":  "..",
		"value dot":        ".",
		"value backslash":  `a\b`,
		"value empty":      "",
	}
	for name, val := range cases {
		t.Run(name, func(t *testing.T) {
			wk := map[string]string{"ProjectName": val}
			if _, err := renderSegment("__ProjectName__", wk); err == nil {
				t.Fatalf("expected error for value %q, got nil", val)
			}
		})
	}
}

func TestRenderRelPath(t *testing.T) {
	wk := map[string]string{"ProjectName": "myapp"}
	got, err := renderRelPath("cmd/__ProjectName__/__ProjectName__.go", wk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "cmd/myapp/myapp.go"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExecute_RendersPathTokensAndProjectName(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	// A directory and a file name both carry the __ProjectName__ token, and the
	// content references the centralized ProjectName variable (no cmd injection).
	writeTemplateFile(t, filepath.Join(srcDir, "template"),
		filepath.Join("cmd", "__ProjectName__", "__ProjectName__.go"),
		"package {{ .ProjectName }}")

	pieces := []*resolver.ResolvedPiece{newTestPiece(srcDir)}
	e := newTestEngine(t)
	if err := e.Execute(targetDir, pieces, nil, nil); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	expected := filepath.Base(targetDir)
	dest := filepath.Join(targetDir, "cmd", expected, expected+".go")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read %s: %v", dest, err)
	}
	if want := "package " + expected; string(got) != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestExecute_UnknownPathTokenErrors(t *testing.T) {
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	writeTemplateFile(t, filepath.Join(srcDir, "template"),
		filepath.Join("__Foo__", "x.go"), "x")

	pieces := []*resolver.ResolvedPiece{newTestPiece(srcDir)}
	e := newTestEngine(t)
	err := e.Execute(targetDir, pieces, nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown path token, got nil")
	}
	if !strings.Contains(err.Error(), "unknown path variable") {
		t.Errorf("error %q does not mention unknown path variable", err.Error())
	}
}
