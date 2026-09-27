package linters

import (
	"context"
	"fmt"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
)

// MissingYear flags albums whose year field is 0 (unset) in the beets database.
type MissingYear struct {
	albums []beets.Album
}

func NewMissingYear(albums []beets.Album) *MissingYear {
	return &MissingYear{albums: albums}
}

func (l *MissingYear) Name() string        { return "missing_year" }
func (l *MissingYear) Description() string { return "Missing Year" }

func (l *MissingYear) Run(ctx context.Context) ([]doctor.Issue, error) {
	var issues []doctor.Issue
	for _, a := range l.albums {
		if ctx.Err() != nil {
			break
		}
		if a.Year == 0 {
			issues = append(issues, doctor.Issue{
				Path:        a.Path,
				AlbumID:     a.ID,
				Description: fmt.Sprintf("%s — %s: year not set", a.AlbumArtist, a.Album),
				Severity:    doctor.SeverityWarning,
			})
		}
	}
	return issues, nil
}
