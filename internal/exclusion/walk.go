package exclusion

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Walk traverses root and returns relative paths of all non-excluded files.
// Directories matching the matcher are skipped entirely (filepath.SkipDir).
// Symlinks are resolved via filepath.EvalSymlinks; both the original and
// resolved path are checked against the matcher.
func Walk(_ context.Context, root string, matcher *Matcher) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("relative path: %w", relErr)
		}
		rel = filepath.ToSlash(rel)

		// Skip root
		if rel == "." {
			return nil
		}

		if matcher != nil {
			// Check original path
			if matcher.Match(rel) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Check resolved symlink path
			if d.Type()&fs.ModeSymlink != 0 {
				resolved, evalErr := filepath.EvalSymlinks(path)
				if evalErr == nil {
					resolvedRel, _ := filepath.Rel(root, resolved)
					resolvedRel = filepath.ToSlash(resolvedRel)
					if matcher.Match(resolvedRel) {
						return nil
					}
				}
			}
		}

		if d.IsDir() {
			return nil
		}

		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		// Non-existent directory is an error
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("walking %s: %w", root, err)
		}
		return nil, fmt.Errorf("walking directory: %w", err)
	}

	return paths, nil
}
