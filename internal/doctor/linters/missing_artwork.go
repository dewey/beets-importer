package linters

import (
	"context"
	"fmt"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
)

// MissingArtwork flags albums that have no artwork path recorded in beets.
type MissingArtwork struct {
	albums []beets.Album
}

func NewMissingArtwork(albums []beets.Album) *MissingArtwork {
	return &MissingArtwork{albums: albums}
}

func (l *MissingArtwork) Name() string        { return "missing_artwork" }
func (l *MissingArtwork) Description() string { return "Missing Artwork" }

func (l *MissingArtwork) Run(ctx context.Context) ([]doctor.Issue, error) {
	var issues []doctor.Issue
	for _, a := range l.albums {
		if ctx.Err() != nil {
			break
		}
		if !a.HasArtwork {
			issues = append(issues, doctor.Issue{
				Path:        a.Path,
				AlbumID:     a.ID,
				Description: fmt.Sprintf("%s — %s: no artwork", a.AlbumArtist, a.Album),
				Severity:    doctor.SeverityWarning,
			})
		}
	}
	return issues, nil
}
