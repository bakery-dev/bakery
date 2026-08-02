// Package engine handles rendering piece templates into target scaffolding directories.
package engine

import (
	"bytes"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/bakery-dev/bakery/internal/resolver"
	"gopkg.in/yaml.v3"
)

// Engine renders piece templates into a target directory.
type Engine struct {
	log *slog.Logger
}

// New creates an Engine.
func New(log *slog.Logger) *Engine {
	return &Engine{log: log}
}

// Execute renders all templates for the given pieces into targetDir.
//
// pieces: resolved pieces in topological order (Dir must be set).
// answers: pieceKey -> answer value map from the prompt module.
// commits: pieceKey -> resolved commit hash map.
func (e *Engine) Execute(targetDir string, pieces []*resolver.ResolvedPiece, answers map[string]any, commits map[string]string) error {
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return fmt.Errorf("resolve target dir: %w", err)
	}

	// Create target directory.
	if err := os.MkdirAll(absTarget, 0o755); err != nil {
		return fmt.Errorf("create target dir %s: %w", absTarget, err)
	}

	// Build template context from answers.
	ctx := e.buildContext(pieces, answers)

	// Process each piece's templates.
	for _, piece := range pieces {
		if piece.Dir == "" {
			e.log.Warn("piece has no dir, skipping", "key", piece.Key)
			continue
		}

		templateDir := filepath.Join(piece.Dir, "template")
		if _, err := os.Stat(templateDir); os.IsNotExist(err) {
			e.log.Debug("no template dir, skipping", "piece", piece.Key)
			continue
		}

		if err := e.processPiece(templateDir, absTarget, ctx); err != nil {
			return fmt.Errorf("process piece %s: %w", piece.Key, err)
		}
	}

	// Write piefile.yaml.
	if err := e.writePiefile(absTarget, pieces, answers); err != nil {
		return fmt.Errorf("write piefile.yaml: %w", err)
	}

	// Write pielock.
	if err := e.writePielock(absTarget, commits); err != nil {
		return fmt.Errorf("write pielock: %w", err)
	}

	return nil
}

// buildContext merges answers into a map usable by text/template,
// and injects a Capabilities map.
func (e *Engine) buildContext(pieces []*resolver.ResolvedPiece, answers map[string]any) map[string]any {
	ctx := make(map[string]any)
	for k, v := range answers {
		ctx[k] = v
	}

	caps := make(map[string]bool)
	for _, p := range pieces {
		if p != nil && p.Piece != nil {
			for _, c := range p.Piece.Provides {
				caps[c] = true
			}
		}
	}
	ctx["Capabilities"] = caps

	return ctx
}

// processPiece walks templateDir, executes each file as a template, and
// writes the result into the sanitized destination under absTarget.
func (e *Engine) processPiece(templateDir, absTarget string, ctx map[string]any) error {
	return filepath.WalkDir(templateDir, func(srcPath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Symlink prohibition.
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", srcPath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink prohibited: %s", srcPath)
		}

		// Skip directories.
		if d.IsDir() {
			return nil
		}

		// Compute relative path.
		rel, err := filepath.Rel(templateDir, srcPath)
		if err != nil {
			return fmt.Errorf("rel path %s: %w", srcPath, err)
		}

		// Sanitize destination.
		dest := filepath.Join(absTarget, rel)
		dest = filepath.Clean(dest)

		// Path traversal check: dest must start with absTarget.
		if !strings.HasPrefix(dest, absTarget+string(os.PathSeparator)) && dest != absTarget {
			return fmt.Errorf("path traversal blocked: %s resolves to %s", srcPath, dest)
		}

		// Read source.
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", srcPath, err)
		}

		// Execute template.
		tmpl, err := template.New(filepath.Base(srcPath)).Funcs(template.FuncMap{
			"indent": func(n int, s string) string {
				pad := strings.Repeat(" ", n)
				return pad + strings.ReplaceAll(s, "\n", "\n"+pad)
			},
		}).Parse(string(data))
		if err != nil {
			return fmt.Errorf("parse template %s: %w", srcPath, err)
		}

		// Check if file exists.
		fileExists := false
		if _, err := os.Stat(dest); err == nil {
			fileExists = true
			if filepath.Base(dest) == ".gitignore" {
				e.log.Debug("appending to existing .gitignore", "path", dest)
			} else {
				e.log.Warn("overwriting existing file", "path", dest)
			}
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, ctx); err != nil {
			return fmt.Errorf("render %s: %w", srcPath, err)
		}

		// Skip if template produced no meaningful content.
		if len(bytes.TrimSpace(buf.Bytes())) == 0 {
			e.log.Debug("template output empty, skipping", "path", dest)
			return nil
		}

		// Create parent dirs.
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", filepath.Dir(dest), err)
		}

		flags := os.O_CREATE | os.O_WRONLY
		if fileExists && filepath.Base(dest) == ".gitignore" {
			flags |= os.O_APPEND
		} else {
			flags |= os.O_TRUNC
		}

		f, err := os.OpenFile(dest, flags, 0o644)
		if err != nil {
			return fmt.Errorf("open %s: %w", dest, err)
		}
		defer func() {
			_ = f.Close()
		}()

		if flags&os.O_APPEND != 0 {
			_, _ = f.WriteString("\n")
		}

		if _, err := f.Write(buf.Bytes()); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}

		e.log.Debug("rendered template", "src", rel, "dest", dest)
		return nil
	})
}

// writePiefile serializes the user's answers as YAML into piefile.yaml.
func (e *Engine) writePiefile(absTarget string, resolvedPieces []*resolver.ResolvedPiece, answers map[string]any) error {
	type piefileEntry struct {
		Answer any `yaml:"answer"`
	}

	type piefile struct {
		Name        string                    `yaml:"name,omitempty"`
		Description string                    `yaml:"description,omitempty"`
		Pieces      map[string]*piefileEntry  `yaml:"pieces"`
	}

	pieces := make(map[string]*piefileEntry, len(resolvedPieces))
	for _, piece := range resolvedPieces {
		ans, ok := answers[piece.Key]
		if !ok {
			ans = true
		}
		pieces[piece.Key] = &piefileEntry{Answer: ans}
	}

	data, err := yaml.Marshal(&piefile{Pieces: pieces})
	if err != nil {
		return fmt.Errorf("marshal piefile: %w", err)
	}

	path := filepath.Join(absTarget, "piefile.yaml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	e.log.Info("wrote piefile.yaml", "path", path)
	return nil
}

// writePielock writes the piece commit hashes as YAML into pielock.
func (e *Engine) writePielock(absTarget string, commits map[string]string) error {
	type pielock struct {
		Resolved map[string]string `yaml:"resolved"`
	}

	data, err := yaml.Marshal(&pielock{Resolved: commits})
	if err != nil {
		return fmt.Errorf("marshal pielock: %w", err)
	}

	path := filepath.Join(absTarget, "pielock")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	e.log.Info("wrote pielock", "path", path)
	return nil
}
