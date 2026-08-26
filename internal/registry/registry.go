package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"golang.org/x/mod/sumdb/dirhash"
	"gopkg.in/yaml.v3"
)

// Registry manages remote git registries: fetching, caching, and reading pieces.
type Registry struct {
	log        *slog.Logger
	dataDir    string
	registries map[string]string // alias -> repo URL
	localDirs  map[string]string // alias -> resolved local absolute path
	commits    map[string]string // alias -> resolved commit hash ("local" for local dirs)
}

// New creates a Registry. Pass a named sub-logger.
func New(log *slog.Logger) *Registry {
	return &Registry{
		log:        log,
		dataDir:    filepath.Join(xdg.DataHome, "bakery", "registries"),
		registries: make(map[string]string),
		localDirs:  make(map[string]string),
		commits:    make(map[string]string),
	}
}

// Update fetches or updates a single registry alias.
// alias: human-readable name (e.g. "core")
// repoURL: git URL (e.g. "user/repo", "github.com/user/repo") OR local path (e.g. "/path/to/repo" or "file:///path/to/repo")
func (r *Registry) Update(alias, repoURL string) error {
	// Local directory fast-path
	if isLocalPath(repoURL) {
		return r.updateLocal(alias, repoURL)
	}

	return r.updateRemote(alias, repoURL)
}

func (r *Registry) updateRemote(alias, repoURL string) error {
	r.registries[alias] = repoURL

	targetDir := r.registryPath(alias)

	// Parse URL to build git-compatible remote
	remote, err := r.buildRemoteURL(repoURL)
	if err != nil {
		return fmt.Errorf("parse repo URL: %w", err)
	}

	// Resolve latest commit hash for HEAD via git ls-remote
	commitHash, err := r.resolveCommit(remote, "HEAD")
	if err != nil {
		return fmt.Errorf("resolve commit for %s: %w", remote, err)
	}
	r.log.Info("resolved commit", "alias", alias, "commit", commitHash)

	r.commits[alias] = commitHash

	// Clone if directory missing
	if _, err := os.Stat(targetDir); os.IsNotExist(err) {
		if err := r.clone(remote, targetDir); err != nil {
			return fmt.Errorf("clone %s: %w", remote, err)
		}
		// Checkout resolved commit
		if err := r.checkout(targetDir, commitHash); err != nil {
			return fmt.Errorf("checkout %s: %w", remote, err)
		}
		r.log.Info("cloned registry", "alias", alias, "path", targetDir)
	} else {
		// Directory exists — check if commit is already available locally
		hasCommit, err := r.hasCommit(targetDir, commitHash)
		if err != nil {
			return fmt.Errorf("check commit in %s: %w", targetDir, err)
		}
		if !hasCommit {
			if err := r.fetch(remote, targetDir); err != nil {
				return fmt.Errorf("fetch %s: %w", remote, err)
			}
			if err := r.checkout(targetDir, commitHash); err != nil {
				return fmt.Errorf("checkout %s: %w", remote, err)
			}
			r.log.Info("fetched registry", "alias", alias, "path", targetDir)
		} else {
			r.log.Debug("registry up to date", "alias", alias)
		}
	}

	// Verify pieces.lock
	if err := verifyPiecesLock(targetDir); err != nil {
		return fmt.Errorf("verify pieces.lock for %s: %w", alias, err)
	}

	return nil
}

func (r *Registry) updateLocal(alias, repoURL string) error {
	localPath := stripFilePrefix(repoURL)

	// Resolve to absolute path
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("resolve local path %s: %w", localPath, err)
	}

	// Verify directory exists
	if info, err := os.Stat(abs); err != nil {
		return fmt.Errorf("local registry %s not found: %w", abs, err)
	} else if !info.IsDir() {
		return fmt.Errorf("local registry %s is not a directory", abs)
	}

	// Strict: verify pieces.lock for local registries too.
	if err := verifyPiecesLock(abs); err != nil {
		return fmt.Errorf("verify pieces.lock for local %s: %w", alias, err)
	}

	// Compute dirhash of pieces.lock itself as the commit hash.
	lockHash, err := computeLockHash(abs)
	if err != nil {
		return fmt.Errorf("compute lock hash for %s: %w", alias, err)
	}

	r.localDirs[alias] = abs
	r.registries[alias] = repoURL
	r.commits[alias] = lockHash
	r.log.Info("local registry registered", "alias", alias, "path", abs, "lock_hash", lockHash)

	return nil
}

// ListRegistries returns all registered aliases and their URLs.
func (r *Registry) ListRegistries() map[string]string {
	out := make(map[string]string, len(r.registries))
	for k, v := range r.registries {
		out[k] = v
	}
	return out
}

// GetCommit returns the resolved commit hash for a URL.
// Returns "local" for local directory registries (actually lockHash).
func (r *Registry) GetCommit(target string) string {
	if commit, ok := r.commits[target]; ok {
		return commit
	}
	alias := r.aliasForURL(target)
	if alias == "" {
		return ""
	}
	return r.commits[alias]
}

// RegistryPath returns the local path for a registry alias.
// For local registries, returns the resolved filesystem path.
// For remote registries, returns the XDG cache path.
func (r *Registry) RegistryPath(alias string) string {
	if p, ok := r.localDirs[alias]; ok {
		return p
	}
	return r.registryPath(alias)
}

// PiecesDir returns the pieces/ subdirectory path for an alias.
func (r *Registry) PiecesDir(alias string) string {
	return filepath.Join(r.RegistryPath(alias), "pieces")
}

// PiesDir returns the pies/ subdirectory path for an alias.
func (r *Registry) PiesDir(alias string) string {
	return filepath.Join(r.RegistryPath(alias), "pies")
}

// FindPie searches all registered registries for a pie matching pieName.
// Returns the first match found. Pie files are expected as <name>.yaml
// inside each registry's pies/ directory.
func (r *Registry) FindPie(pieName string) (*Pie, error) {
	target := pieName + ".yaml"

	// Search remote registries.
	for alias := range r.registries {
		if p, ok := r.findPieIn(alias, target); ok {
			return p, nil
		}
	}

	// Search local registries.
	for alias := range r.localDirs {
		if p, ok := r.findPieIn(alias, target); ok {
			return p, nil
		}
	}

	return nil, fmt.Errorf("pie %q not found in any registered registry", pieName)
}

func (r *Registry) findPieIn(alias, filename string) (*Pie, bool) {
	dir := r.PiesDir(alias)
	path := filepath.Join(dir, filename)
	p, err := LoadPie(path)
	if err == nil {
		return p, true
	}
	return nil, false
}

// --- internal helpers ---

func (r *Registry) registryPath(alias string) string {
	return filepath.Join(r.dataDir, alias)
}

// buildRemoteURL converts shorthand like "user/repo" or "github.com/user/repo" to a git-fetchable URL.
// Bare "org/repo" shorthand (no host) is assumed to be on github.com.
func (r *Registry) buildRemoteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)

	// Already a full git URL
	if strings.HasPrefix(raw, "git@") || strings.HasPrefix(raw, "https://") ||
		strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "ssh://") ||
		strings.HasPrefix(raw, "git://") {
		return raw, nil
	}

	// GitHub shorthand without host: "org/repo" → "github.com/org/repo".
	// Detected when the first path segment has no dot (i.e. no hostname like github.com).
	if !strings.Contains(raw, "://") {
		first := raw
		if i := strings.Index(raw, "/"); i >= 0 {
			first = raw[:i]
		}
		if !strings.Contains(first, ".") {
			raw = "github.com/" + raw
		}
	}

	// Shorthand: github.com/user/repo → https://github.com/user/repo.git
	if !strings.Contains(raw, ".git") {
		raw += ".git"
	}
	if !strings.Contains(raw, "://") {
		return "https://" + raw, nil
	}
	return raw, nil
}

// resolveCommit runs `git ls-remote <remote> <ref>` and extracts the SHA.
func (r *Registry) resolveCommit(remote, ref string) (string, error) {
	cmd := exec.Command("git", "ls-remote", remote, ref)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git ls-remote %s %s: %w (output: %s)", remote, ref, err, string(out))
	}
	line := strings.TrimSpace(string(out))
	parts := strings.Split(line, "\t")
	if len(parts) < 1 {
		return "", fmt.Errorf("empty ls-remote output for %s %s", remote, ref)
	}
	return parts[0], nil
}

// clone runs `git clone <remote> <dir>`.
func (r *Registry) clone(remote, dir string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", remote, dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("git clone: %w (output: %s)", err, string(out))
	}
	if len(out) > 0 {
		r.log.Debug(string(out))
	}
	return nil
}

// fetch runs `git fetch <remote>` inside <dir>.
func (r *Registry) fetch(remote, dir string) error {
	cmd := exec.Command("git", "-C", dir, "fetch", remote)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git fetch: %w (output: %s)", err, string(out))
	}
	return nil
}

// checkout runs `git checkout <commit>` inside <dir>.
func (r *Registry) checkout(dir, commit string) error {
	cmd := exec.Command("git", "-C", dir, "checkout", commit)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git checkout %s: %w (output: %s)", commit, err, string(out))
	}
	return nil
}

// hasCommit checks if a commit SHA exists in the local repo's object store.
func (r *Registry) hasCommit(dir, commit string) (bool, error) {
	cmd := exec.Command("git", "-C", dir, "cat-file", "-t", commit)
	_, err := cmd.CombinedOutput()
	return err == nil, nil
}

// verifyPiecesLock reads pieces.lock (YAML map of pieceName -> dirhash) and
// verifies each piece directory's hash. Returns error if lock is missing or
// any hash mismatches. pies/ is excluded from locking.
func verifyPiecesLock(dir string) error {
	lockPath := filepath.Join(dir, "pieces.lock")

	// Read lock file.
	lockData, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("pieces.lock not found in %s", dir)
		}
		return fmt.Errorf("read pieces.lock: %w", err)
	}

	// Parse YAML: map[string]string  (pieceName -> dirhash)
	var lockMap map[string]string
	if err := yaml.Unmarshal(lockData, &lockMap); err != nil {
		return fmt.Errorf("parse pieces.lock: %w", err)
	}

	// Track which lock entries were verified.
	verified := make(map[string]bool)

	// Verify each entry in the lockfile.
	piecesDir := filepath.Join(dir, "pieces")
	for pieceName, expectedHash := range lockMap {
		piecePath := filepath.Join(piecesDir, pieceName)
		actualHash, err := dirhash.HashDir(piecePath, "", dirhash.DefaultHash)
		if err != nil {
			return fmt.Errorf("hash dir %s: %w", pieceName, err)
		}
		if actualHash != expectedHash {
			return fmt.Errorf("hash mismatch for piece %q: expected %s, got %s", pieceName, expectedHash, actualHash)
		}
		verified[pieceName] = true
	}

	// Check for pieces on disk that are not in the lock.
	entries, err := os.ReadDir(piecesDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No pieces/ dir — lock must be empty.
			if len(lockMap) > 0 {
				return fmt.Errorf("pieces/ directory missing but lock has %d entries", len(lockMap))
			}
			return nil
		}
		return fmt.Errorf("read pieces/: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !verified[name] {
			return fmt.Errorf("piece %q on disk but not in pieces.lock", name)
		}
	}

	return nil
}

// computeLockHash reads pieces.lock and returns its SHA256 hex digest.
func computeLockHash(dir string) (string, error) {
	lockPath := filepath.Join(dir, "pieces.lock")
	return fileSHA256(lockPath)
}

// fileSHA256 computes the hex-encoded SHA256 of a file.
func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}

// isLocalPath returns true if the URL is a local filesystem reference.
func isLocalPath(raw string) bool {
	return strings.HasPrefix(raw, "file://") || filepath.IsAbs(raw)
}

// stripFilePrefix removes the "file://" scheme if present.
func stripFilePrefix(raw string) string {
	if strings.HasPrefix(raw, "file://") {
		return raw[7:]
	}
	return raw
}

// --- RegistryAPI interface implementation ---

// AllPieces loads every piece from all registered registries.
// Keys are "url:name".
func (r *Registry) AllPieces() map[string]*Piece {
	pieces := make(map[string]*Piece)
	aliases := r.allAliases()
	for _, alias := range aliases {
		r.loadPiecesFor(alias, pieces)
	}
	return pieces
}

// GetPiece loads a single piece by alias:name.
func (r *Registry) GetPiece(alias, name string) (*Piece, error) {
	piecesDir := r.PiecesDir(alias)
	piecePath := filepath.Join(piecesDir, name, "piece.yml")
	return LoadPiece(piecePath)
}

// PieceDir returns the on-disk directory for a piece (alias:name).
func (r *Registry) PieceDir(alias, name string) string {
	return filepath.Join(r.PiecesDir(alias), name)
}

func (r *Registry) aliasForURL(url string) string {
	for a, u := range r.registries {
		if u == url {
			return a
		}
	}
	return ""
}

// FindByCapability returns all pieces that provide the given capability.
func (r *Registry) FindByCapability(capability string) ([]*Piece, error) {
	all := r.AllPieces()
	var results []*Piece
	for _, p := range all {
		for _, c := range p.Provides {
			if c == capability {
				results = append(results, p)
				break
			}
		}
	}
	return results, nil
}

func (r *Registry) allAliases() []string {
	seen := make(map[string]bool)
	var aliases []string
	for alias := range r.registries {
		if !seen[alias] {
			aliases = append(aliases, alias)
			seen[alias] = true
		}
	}
	return aliases
}

func (r *Registry) loadPiecesFor(alias string, into map[string]*Piece) {
	piecesDir := r.PiecesDir(alias)
	entries, err := os.ReadDir(piecesDir)
	if err != nil {
		return // directory may not exist yet
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		piecePath := filepath.Join(piecesDir, name, "piece.yml")
		piece, err := LoadPiece(piecePath)
		if err != nil {
			continue
		}
		// Key by alias (not url) so piece refs are stable regardless of whether
		// the alias points at a remote URL or a local path.
		key := alias + ":" + name
		into[key] = piece
	}
}
