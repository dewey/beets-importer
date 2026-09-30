package linters

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/dewey/beets-importer/internal/doctor"
	"golang.org/x/text/unicode/norm"
)

// DuplicateNames flags directories that hold two entries with the same name
// in different Unicode forms, like "ö" as one code point (NFC) and as "o"
// plus a combining mark (NFD). This happens when a Linux tool (e.g. a torrent
// client) writes NFC names next to NFD files that were copied from a Mac. Over
// SMB, macOS lists both entries but can only open one of them, so beets sees
// extra "unmatched" tracks. The fix must run on the file server.
type DuplicateNames struct {
	roots []string
}

func NewDuplicateNames(roots ...string) *DuplicateNames {
	return &DuplicateNames{roots: roots}
}

func (l *DuplicateNames) Name() string        { return "duplicate_names" }
func (l *DuplicateNames) Description() string { return "Duplicate Unicode Names" }

func (l *DuplicateNames) Run(ctx context.Context) ([]doctor.Issue, error) {
	var issues []doctor.Issue
	for _, root := range l.roots {
		if err := walkDuplicateNames(ctx, os.DirFS(root), root, ".", &issues); err != nil {
			return nil, err
		}
	}
	return issues, nil
}

// walkDuplicateNames only reads directories and never opens files, so it
// stays fast on large folders.
func walkDuplicateNames(ctx context.Context, fsys fs.FS, root, dir string, issues *[]doctor.Issue) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(entries))
	var dups []string
	for _, e := range entries {
		key := norm.NFC.String(e.Name())
		if seen[key] {
			dups = append(dups, key)
			// Both entries open the same directory, so walk it only once.
			continue
		}
		seen[key] = true
		if e.IsDir() {
			if err := walkDuplicateNames(ctx, fsys, root, path.Join(dir, e.Name()), issues); err != nil {
				return err
			}
		}
	}
	if len(dups) > 0 {
		*issues = append(*issues, doctor.Issue{
			Path:        filepath.Join(root, filepath.FromSlash(dir)),
			Description: fmt.Sprintf("%d name(s) stored twice in different Unicode forms: %s", len(dups), strings.Join(dups, ", ")),
			Severity:    doctor.SeverityError,
		})
	}
	return nil
}
