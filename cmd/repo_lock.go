package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/mod/sumdb/dirhash"
	"gopkg.in/yaml.v3"
)

var repoLockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Generate pieces.lock from the current directory's pieces/",
	Long: `Scan pieces/ in the current working directory, compute a stable
directory hash for each piece subdirectory using dirhash.HashDir,
and write the results to pieces.lock as a YAML map.

Pies (pies/) are excluded from locking.

Example:
  cd my-registry
  bakery repo lock`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runRepoLock,
}

func runRepoLock(_ *cobra.Command, _ []string) error {
	piecesDir := filepath.Join(".", "pieces")

	entries, err := os.ReadDir(piecesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("pieces/ directory not found in current directory")
		}
		return fmt.Errorf("read pieces/: %w", err)
	}

	hashes := make(map[string]string)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pieceName := entry.Name()
		piecePath := filepath.Join(piecesDir, pieceName)

		hash, err := dirhash.HashDir(piecePath, "", dirhash.DefaultHash)
		if err != nil {
			return fmt.Errorf("hash piece %q: %w", pieceName, err)
		}

		hashes[pieceName] = hash
		fmt.Printf("hashed: %-20s %s\n", pieceName, hash)
	}

	data, err := yaml.Marshal(hashes)
	if err != nil {
		return fmt.Errorf("marshal lock: %w", err)
	}

	lockPath := "pieces.lock"
	if err := os.WriteFile(lockPath, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", lockPath, err)
	}

	fmt.Printf("\nwrote %s (%d pieces)\n", lockPath, len(hashes))
	return nil
}

func init() {
	repoCmd.AddCommand(repoLockCmd)
}
