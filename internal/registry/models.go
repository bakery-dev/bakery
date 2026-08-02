// Package registry manages pieces and registries.
package registry

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Piece matches the piece.yml schema.
type Piece struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Author      string   `yaml:"author"`
	Provides    []string `yaml:"provides,omitempty"`
	DependsOn   []string `yaml:"depends_on,omitempty"`
	Actions     []string `yaml:"actions,omitempty"`
	Prompt      *Prompt  `yaml:"prompt,omitempty"`
}

// Prompt defines the interactive question for a piece.
type Prompt struct {
	Type    string      `yaml:"type"` // confirm, select, multi_select, input
	Title   string      `yaml:"title"`
	Default interface{} `yaml:"default,omitempty"`
	Options []Option    `yaml:"options,omitempty"`
}

// Option is a choice for select / multi_select prompts.
type Option struct {
	Label string      `yaml:"label"`
	Value interface{} `yaml:"value"`
}

// Pie matches the piefile.yaml schema.
type Pie struct {
	Name        string                    `yaml:"name"`
	Description string                    `yaml:"description"`
	Pieces      map[string]*PieEntry      `yaml:"pieces"`
}

// PieEntry holds a referenced piece and its pre-filled answer.
type PieEntry struct {
	Answer *bool `yaml:"answer"` // nil means CLI should prompt
}

// LoadPiece parses a piece.yml file from disk.
func LoadPiece(path string) (*Piece, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read piece file %s: %w", path, err)
	}

	var p Piece
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse piece file %s: %w", path, err)
	}
	return &p, nil
}

// LoadPie parses a piefile.yaml file from disk.
func LoadPie(path string) (*Pie, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pie file %s: %w", path, err)
	}

	var pie Pie
	if err := yaml.Unmarshal(data, &pie); err != nil {
		return nil, fmt.Errorf("parse pie file %s: %w", path, err)
	}

	// YAML maps an empty pie entry (e.g. "alias:piece:" with no body) to a nil
	// *PieEntry. Normalize so a missing answer consistently means "prompt".
	for k, v := range pie.Pieces {
		if v == nil {
			pie.Pieces[k] = &PieEntry{}
		}
	}

	return &pie, nil
}
