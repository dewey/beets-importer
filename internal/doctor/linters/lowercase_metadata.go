package linters

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
)

// LowercaseMetadata flags tracks where artist, album, and title are all
// entirely lowercase. This pattern is a hallmark of old auto-tagged imports
// (e.g. early MusicBrainz taggers) and is a reliable hint that the metadata
// needs re-tagging with a modern source.
type LowercaseMetadata struct {
	items []beets.Item
}

func NewLowercaseMetadata(items []beets.Item) *LowercaseMetadata {
	return &LowercaseMetadata{items: items}
}

func (l *LowercaseMetadata) Name() string        { return "lowercase_metadata" }
func (l *LowercaseMetadata) Description() string { return "Lowercase Metadata" }

// isAllLowercase returns true when s is non-empty and every letter is lowercase.
func isAllLowercase(s string) bool {
	return s != "" && s == strings.ToLower(s) && strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyz")
}

func (l *LowercaseMetadata) Run(ctx context.Context) ([]doctor.Issue, error) {
	var issues []doctor.Issue
	for _, item := range l.items {
		if ctx.Err() != nil {
			break
		}
		if isAllLowercase(item.Artist) && isAllLowercase(item.Album) && isAllLowercase(item.Title) {
			issues = append(issues, doctor.Issue{
				Path:        item.Path,
				Description: fmt.Sprintf("%s: artist/album/title are all lowercase — likely old auto-tag", filepath.Base(item.Path)),
				Severity:    doctor.SeverityWarning,
			})
		}
	}
	return issues, nil
}
