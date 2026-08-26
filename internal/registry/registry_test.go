package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"log/slog"

	"golang.org/x/mod/sumdb/dirhash"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// setupValidLocalRegistry creates a temp dir with pieces/ subdir and a valid
// pieces.lock (JSON map of pieceName -> dirhash) so verifyPiecesLock passes.
func setupValidLocalRegistry(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create pieces/demo/ with a piece.yml file.
	pieceDir := filepath.Join(dir, "pieces", "demo")
	if err := os.MkdirAll(pieceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pieceDir, "piece.yml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Compute dirhash for the piece directory.
	hash, err := dirhash.HashDir(pieceDir, "", dirhash.DefaultHash)
	if err != nil {
		t.Fatal(err)
	}

	lockMap := map[string]string{
		"demo": hash,
	}
	lockData, err := json.MarshalIndent(lockMap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pieces.lock"), append(lockData, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestAllPiecesKeyedByAlias(t *testing.T) {
	tmpDir := setupValidLocalRegistry(t)
	reg := New(newTestLogger())

	const alias = "myalias"
	if err := reg.Update(alias, tmpDir); err != nil {
		t.Fatalf("Update: %v", err)
	}

	all := reg.AllPieces()
	// Key must be alias:name, NOT the filesystem path, so piece refs stay
	// stable whether the alias points at a local path or a remote URL.
	if _, ok := all[alias+":demo"]; !ok {
		t.Errorf("AllPieces keys = %v, want key %q", keys(all), alias+":demo")
	}
	for k := range all {
		if strings.Contains(k, tmpDir) {
			t.Errorf("piece key %q must not embed the registered path", k)
		}
	}
}

func keys(m map[string]*Piece) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestBuildRemoteURL(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		// Bare org/repo shorthand → assumed github.com.
		{"bakery-dev/bakery", "https://github.com/bakery-dev/bakery.git"},
		// Explicit host shorthand.
		{"github.com/user/repo", "https://github.com/user/repo.git"},
		// Full URLs passed through unchanged.
		{"https://github.com/user/repo", "https://github.com/user/repo"},
		{"git@github.com:user/repo.git", "git@github.com:user/repo.git"},
	}

	reg := New(newTestLogger())
	for _, tt := range tests {
		got, err := reg.buildRemoteURL(tt.in)
		if err != nil {
			t.Errorf("buildRemoteURL(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("buildRemoteURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"/home/user/my-registry", true},
		{"file:///home/user/my-registry", true},
		{"./relative-path", false}, // not absolute, not file://
		{"github.com/user/repo", false},
		{"https://github.com/user/repo", false},
		{"git@github.com:user/repo.git", false},
	}

	for _, tt := range tests {
		got := isLocalPath(tt.url)
		if got != tt.want {
			t.Errorf("isLocalPath(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestStripFilePrefix(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"file:///home/user/repo", "/home/user/repo"},
		{"/home/user/repo", "/home/user/repo"},
		{"file:///tmp/reg", "/tmp/reg"},
	}

	for _, tt := range tests {
		got := stripFilePrefix(tt.in)
		if got != tt.want {
			t.Errorf("stripFilePrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUpdateLocal(t *testing.T) {
	tmpDir := setupValidLocalRegistry(t)

	log := newTestLogger()
	reg := New(log)

	// Register via absolute path
	err := reg.Update("test-local", tmpDir)
	if err != nil {
		t.Fatalf("Update local path: %v", err)
	}

	// Verify path resolution
	if got := reg.RegistryPath("test-local"); got != tmpDir {
		t.Errorf("RegistryPath = %q, want %q", got, tmpDir)
	}

	// Verify commit is a 64-char SHA256 hex string (hash of pieces.lock).
	commit := reg.GetCommit("test-local")
	if len(commit) != 64 {
		t.Errorf("GetCommit = %q (len=%d), want 64-char sha256 hex", commit, len(commit))
	}

	// Verify PiecesDir/PiesDir resolve correctly
	expectedPieces := filepath.Join(tmpDir, "pieces")
	if got := reg.PiecesDir("test-local"); got != expectedPieces {
		t.Errorf("PiecesDir = %q, want %q", got, expectedPieces)
	}
}

func TestUpdateLocalViaFilePrefix(t *testing.T) {
	tmpDir := setupValidLocalRegistry(t)

	log := newTestLogger()
	reg := New(log)

	fileURL := "file://" + tmpDir
	err := reg.Update("file-local", fileURL)
	if err != nil {
		t.Fatalf("Update file:// path: %v", err)
	}

	commit := reg.GetCommit("file-local")
	if len(commit) != 64 {
		t.Errorf("GetCommit = %q (len=%d), want 64-char sha256 hex", commit, len(commit))
	}
}

func TestUpdateLocalNotFound(t *testing.T) {
	log := newTestLogger()
	reg := New(log)

	err := reg.Update("missing", "/nonexistent/path/to/registry")
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
}

func TestUpdateLocalNotDir(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "bakery-local-file-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	log := newTestLogger()
	reg := New(log)

	err = reg.Update("notdir", tmpFile.Name())
	if err == nil {
		t.Error("expected error for non-directory path, got nil")
	}
}

func TestUpdateLocalMissingLock(t *testing.T) {
	// Temp dir with no pieces.lock — must fail.
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "pieces"), 0o755); err != nil {
		t.Fatal(err)
	}

	log := newTestLogger()
	reg := New(log)

	err := reg.Update("no-lock", tmpDir)
	if err == nil {
		t.Error("expected error for missing pieces.lock, got nil")
	}
}

func TestUpdateLocalHashMismatch(t *testing.T) {
	tmpDir := setupValidLocalRegistry(t)

	// Tamper with a tracked file.
	pieceFile := filepath.Join(tmpDir, "pieces", "demo", "piece.yml")
	if err := os.WriteFile(pieceFile, []byte("name: tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	log := newTestLogger()
	reg := New(log)

	err := reg.Update("bad-hash", tmpDir)
	if err == nil {
		t.Error("expected hash mismatch error, got nil")
	}
}
