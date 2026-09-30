package linters

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
)

// SplitAlbums flags beets albums whose tracks do not agree on album artist,
// album name, or MusicBrainz album ID, or that disagree with the album row.
// Players like Navidrome group tracks by these tags, so they show such an
// album twice or split it by track artist.
type SplitAlbums struct {
	albums []beets.Album
	items  []beets.Item
}

func NewSplitAlbums(albums []beets.Album, items []beets.Item) *SplitAlbums {
	return &SplitAlbums{albums: albums, items: items}
}

func (l *SplitAlbums) Name() string        { return "split_albums" }
func (l *SplitAlbums) Description() string { return "Split Albums" }

type albumTags struct {
	albumArtist, album, mbAlbumID string
}

func (t albumTags) String() string {
	id := t.mbAlbumID
	if id == "" {
		id = "no MusicBrainz ID"
	}
	return fmt.Sprintf("%q / %q / %s", t.albumArtist, t.album, id)
}

func (l *SplitAlbums) Run(ctx context.Context) ([]doctor.Issue, error) {
	tags := make(map[int]map[albumTags]int)
	for _, it := range l.items {
		if it.AlbumID == 0 {
			continue
		}
		if tags[it.AlbumID] == nil {
			tags[it.AlbumID] = make(map[albumTags]int)
		}
		tags[it.AlbumID][albumTags{it.AlbumArtist, it.Album, it.MBAlbumID}]++
	}

	var issues []doctor.Issue
	for _, a := range l.albums {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		groups := tags[a.ID]
		var desc string
		switch {
		case len(groups) > 1:
			var parts []string
			for t, n := range groups {
				parts = append(parts, fmt.Sprintf("%d× %s", n, t))
			}
			sort.Strings(parts)
			desc = fmt.Sprintf("%s — %s: tracks disagree: %s", a.AlbumArtist, a.Album, strings.Join(parts, ", "))
		case len(groups) == 1:
			for t := range groups {
				if t.albumArtist != a.AlbumArtist || t.album != a.Album {
					desc = fmt.Sprintf("%s — %s: tracks are tagged %q / %q", a.AlbumArtist, a.Album, t.albumArtist, t.album)
				}
			}
		}
		if desc == "" {
			continue
		}
		issues = append(issues, doctor.Issue{
			Path:        a.Path,
			AlbumID:     a.ID,
			Description: desc,
			Severity:    doctor.SeverityWarning,
		})
	}
	return issues, nil
}
