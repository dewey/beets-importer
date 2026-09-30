package linters

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
)

// ProtectedAudio flags tracks with FairPlay DRM from the iTunes Store. Only
// iTunes can play them, so Navidrome and other players cannot. beets reports
// their format as plain AAC, but Apple gives these files the .m4p extension.
type ProtectedAudio struct {
	items []beets.Item
}

func NewProtectedAudio(items []beets.Item) *ProtectedAudio {
	return &ProtectedAudio{items: items}
}

func (l *ProtectedAudio) Name() string        { return "protected_audio" }
func (l *ProtectedAudio) Description() string { return "Protected (DRM) Audio" }

func (l *ProtectedAudio) Run(ctx context.Context) ([]doctor.Issue, error) {
	var issues []doctor.Issue
	for _, it := range l.items {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if strings.ToLower(filepath.Ext(it.Path)) != ".m4p" {
			continue
		}
		issues = append(issues, doctor.Issue{
			Path:        it.Path,
			AlbumID:     it.AlbumID,
			Description: fmt.Sprintf("%s — %s: protected iTunes file, only iTunes can play it", it.Artist, it.Title),
			Severity:    doctor.SeverityError,
		})
	}
	return issues, nil
}
