package linters

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
)

// audioExtensions is the set of file extensions that count as audio files.
var audioExtensions = map[string]bool{
	".mp3": true, ".flac": true, ".m4a": true, ".aac": true,
	".ogg": true, ".opus": true, ".wav": true, ".aiff": true,
	".ape": true, ".wv": true,
}

// UntrackedDirs flags directories inside the library root that contain audio
// files but no track known to the beets database. This usually means the
// album was never imported, or was removed from the DB without deleting the
// files.
type UntrackedDirs struct {
	libraryRoot string
	items       []beets.Item
}

func NewUntrackedDirs(libraryRoot string, items []beets.Item) *UntrackedDirs {
	return &UntrackedDirs{libraryRoot: libraryRoot, items: items}
}

func (l *UntrackedDirs) Name() string        { return "untracked_dirs" }
func (l *UntrackedDirs) Description() string { return "Untracked Directories" }

func (l *UntrackedDirs) Run(ctx context.Context) ([]doctor.Issue, error) {
	// Album paths only point at one folder, so multi-disc albums and
	// singletons would show up as untracked. Track folders cover all cases.
	known := make(map[string]bool)
	for _, it := range l.items {
		if it.Path != "" {
			known[filepath.Dir(filepath.Clean(it.Path))] = true
		}
	}

	var issues []doctor.Issue
	err := filepath.WalkDir(l.libraryRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		hasAudio := false
		hasSubdirs := false
		for _, e := range entries {
			if e.IsDir() {
				hasSubdirs = true
			} else if audioExtensions[strings.ToLower(filepath.Ext(e.Name()))] {
				hasAudio = true
			}
		}
		if hasAudio {
			if !known[filepath.Clean(path)] {
				issues = append(issues, doctor.Issue{
					Path:        path,
					Description: "contains audio files but is not tracked in the beets database",
					Severity:    doctor.SeverityError,
				})
			}
			return fs.SkipDir
		}
		if !hasSubdirs {
			return fs.SkipDir
		}
		return nil
	})
	return issues, err
}
