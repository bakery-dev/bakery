// Package resolver performs topological sorting and dependency resolution for pies.
package resolver

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/bakery-dev/bakery/internal/registry"
)

// ---------- public types ----------

// RegistryAPI is the interface the resolver uses to query the registry.
type RegistryAPI interface {
	GetPiece(alias, name string) (*registry.Piece, error)
	FindByCapability(capability string) ([]*registry.Piece, error)
	AllPieces() map[string]*registry.Piece
	PieceDir(alias, name string) string
}

// ResolutionContext is the output of a successful resolution.
type ResolutionContext struct {
	Pie            *registry.Pie
	Pieces         []*ResolvedPiece
	PieceMap       map[string]*ResolvedPiece
	PendingPrompts []*PromptItem
}

// ResolvedPiece wraps a Piece with its registry key and on-disk path.
type ResolvedPiece struct {
	Key   string // "alias:name"
	Dir   string // absolute path to the piece directory (for template rendering)
	Piece *registry.Piece
}

// PromptItem carries enough info for the prompt module to render a question.
type PromptItem struct {
	PieceKey string
	Piece    *registry.Piece
	Prompt   *registry.Prompt
}

// ---------- resolver ----------

// Resolver resolves pie dependencies and orders pieces topologically.
type Resolver struct {
	log *slog.Logger
	reg RegistryAPI
}

// New creates a new Resolver.
func New(log *slog.Logger, reg RegistryAPI) *Resolver {
	return &Resolver{log: log, reg: reg}
}

// Resolve performs a topological sort of all pieces required by the given Pie.
func (r *Resolver) Resolve(pie *registry.Pie) (*ResolutionContext, error) {
	allPieces := r.reg.AllPieces()

	// resolved tracks visited pieces by key.
	resolved := make(map[string]*ResolvedPiece)
	// adj[from] = list of dependency keys (from depends on each dep).
	adj := make(map[string][]string)
	// in-progress set for cycle detection.
	inProgress := make(map[string]bool)
	// capability → pieceKey (for collision detection).
	capabilityProvider := make(map[string]string)

	var resolve func(key string) error
	resolve = func(key string) error {
		if inProgress[key] {
			return fmt.Errorf("circular dependency detected involving %s", key)
		}
		if _, ok := resolved[key]; ok {
			return nil
		}
		inProgress[key] = true
		defer func() { inProgress[key] = false }()

		piece, ok := allPieces[key]
		if !ok {
			return fmt.Errorf("piece %q not found in any registry", key)
		}

		// Resolve on-disk directory.
		alias, name, _ := parseRef(key)
		dir := r.reg.PieceDir(alias, name)

		resolved[key] = &ResolvedPiece{Key: key, Dir: dir, Piece: piece}

		// Register capabilities — detect collisions.
		for _, cap := range piece.Provides {
			if existing, dup := capabilityProvider[cap]; dup {
				return fmt.Errorf("capability collision: %q provided by both %q and %q", cap, existing, key)
			}
			capabilityProvider[cap] = key
		}

		// Build adjacency and recurse.
		adj[key] = make([]string, 0, len(piece.DependsOn))
		for _, depStr := range piece.DependsOn {
			depKey, err := r.resolveRef(depStr, key, capabilityProvider, allPieces, resolved, adj, inProgress)
			if err != nil {
				return err
			}
			adj[key] = append(adj[key], depKey)
			if err := resolve(depKey); err != nil {
				return err
			}
		}

		return nil
	}

	// Seed from pie entries.
	for ref := range pie.Pieces {
		alias, name, err := parseRef(ref)
		if err != nil {
			return nil, fmt.Errorf("parse pie piece ref %q: %w", ref, err)
		}
		if err := resolve(alias + ":" + name); err != nil {
			return nil, err
		}
	}

	// Topological sort.
	sorted := topoSort(resolved, adj)

	// Collect pending prompts.
	var prompts []*PromptItem
	for _, rp := range sorted {
		if rp.Piece.Prompt == nil {
			continue
		}
		entry, has := pie.Pieces[rp.Key]
		if has && entry.Answer != nil {
			continue
		}
		prompts = append(prompts, &PromptItem{
			PieceKey: rp.Key,
			Piece:    rp.Piece,
			Prompt:   rp.Piece.Prompt,
		})
	}

	// Build PieceMap.
	pieceMap := make(map[string]*ResolvedPiece, len(resolved))
	for k, v := range resolved {
		pieceMap[k] = v
	}

	return &ResolutionContext{
		Pie:            pie,
		Pieces:         sorted,
		PieceMap:       pieceMap,
		PendingPrompts: prompts,
	}, nil
}

// resolveRef maps a dependency string to a concrete "alias:name" key.
func (r *Resolver) resolveRef(depStr, requester string,
	capabilityProvider map[string]string,
	allPieces map[string]*registry.Piece,
	_ map[string]*ResolvedPiece,
	_ map[string][]string,
	_ map[string]bool,
) (string, error) {

	// Try direct piece ref first (alias:name).
	if alias, name, ok := tryParsePieceRef(depStr); ok {
		key := alias + ":" + name
		if _, exists := allPieces[key]; !exists {
			return "", fmt.Errorf("piece %q not found (referenced by %s)", depStr, requester)
		}
		return key, nil
	}

	// Try capability lookup in already-resolved pieces.
	if provider, ok := capabilityProvider[depStr]; ok {
		return provider, nil
	}

	// Fall back to registry scan.
	for key, piece := range allPieces {
		for _, c := range piece.Provides {
			if c == depStr {
				return key, nil
			}
		}
	}

	return "", fmt.Errorf("dependency %q not found as piece or capability (referenced by %s)", depStr, requester)
}

// tryParsePieceRef checks if s looks like "alias:name".
func tryParsePieceRef(s string) (alias, name string, ok bool) {
	idx := strings.LastIndex(s, ":")
	if idx == -1 {
		return "", "", false
	}
	alias = s[:idx]
	name = s[idx+1:]
	if strings.Contains(name, "/") {
		return "", "", false
	}
	return alias, name, true
}

// parseRef extracts alias and name from a pie ref string.
func parseRef(ref string) (string, string, error) {
	idx := strings.LastIndex(ref, ":")
	if idx == -1 {
		return "", "", fmt.Errorf("invalid piece ref %q: expected alias:name", ref)
	}
	return ref[:idx], ref[idx+1:], nil
}

// topoSort runs Kahn's algorithm on the resolved graph.
func topoSort(resolved map[string]*ResolvedPiece, adj map[string][]string) []*ResolvedPiece {
	inDegree := make(map[string]int)
	for key := range resolved {
		inDegree[key] = 0
	}
	for from, deps := range adj {
		for _, dep := range deps {
			if _, ok := resolved[dep]; ok {
				inDegree[from]++
			}
		}
	}

	var queue []string
	for key, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, key)
		}
	}
	slices.Sort(queue)

	var sorted []*ResolvedPiece
	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		sorted = append(sorted, resolved[node])

		for from, deps := range adj {
			for _, dep := range deps {
				if dep == node {
					inDegree[from]--
					if inDegree[from] == 0 {
						queue = append(queue, from)
					}
				}
			}
		}
		slices.Sort(queue)
	}

	return sorted
}
