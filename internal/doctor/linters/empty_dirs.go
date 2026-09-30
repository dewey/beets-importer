package linters

import (
	"context"
	"os"
	"path/filepath"

	"github.com/dewey/beets-importer/internal/doctor"
)

// EmptyDirs flags directories inside the library root that contain no files
// at all (completely empty). These are often left behind after a reorganise
// or a failed import and are safe to remove.
type EmptyDirs struct {
	libraryRoot string
}

func NewEmptyDirs(libraryRoot string) *EmptyDirs {
	return &EmptyDirs{libraryRoot: libraryRoot}
}

func (l *EmptyDirs) Name() string        { return "empty_dirs" }
func (l *EmptyDirs) Description() string { return "Empty Directories" }

func (l *EmptyDirs) Run(ctx context.Context) ([]doctor.Issue, error) {
	var issues []doctor.Issue
	err := walkEmptyDirs(ctx, l.libraryRoot, &issues)
	return issues, err
}

// walkEmptyDirs only reads directories and never opens files, so it stays
// fast on large libraries.
func walkEmptyDirs(ctx context.Context, path string, issues *[]doctor.Issue) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		*issues = append(*issues, doctor.Issue{
			Path:        path,
			Description: "empty directory",
			Severity:    doctor.SeverityWarning,
		})
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if err := walkEmptyDirs(ctx, filepath.Join(path, e.Name()), issues); err != nil {
			return err
		}
	}
	return nil
}
