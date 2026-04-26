package db

import (
	"fmt"
	"path/filepath"
	"sort"
)

// SortedMigrationFiles returns every .sql migration in lexicographic order.
// Migration filenames are zero-padded, so lexical order matches apply order.
func SortedMigrationFiles(dir string) ([]string, error) {
	pattern := filepath.Join(dir, "*.sql")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob migration files: %w", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("no migration files found in %s", dir)
	}
	return files, nil
}
