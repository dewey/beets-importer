package linters

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
	"golang.org/x/text/unicode/norm"
)

// ArtistVariants flags albums whose album artist is written in another way
// on other albums ("Lady GaGa" and "Lady Gaga"), or has the same name but no
// or a different artist ID. Players show each spelling or ID as its own
// artist. The spelling used on albums with a MusicBrainz artist ID is taken
// as the right one, so albums that already use it are not flagged.
type ArtistVariants struct {
	albums []beets.Album
}

func NewArtistVariants(albums []beets.Album) *ArtistVariants {
	return &ArtistVariants{albums: albums}
}

func (l *ArtistVariants) Name() string        { return "artist_variants" }
func (l *ArtistVariants) Description() string { return "Artist Spelling Variants" }

// The discogs and bandcamp plugins also write their own IDs into
// mb_albumartistid, so only a UUID counts as a MusicBrainz ID.
var mbidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// vaName is the beets default for compilations. It is one shared name whether
// or not the album has an artist ID, so flagging it only adds noise.
const vaName = "Various Artists"

var leadingThe = regexp.MustCompile(`^the\s+|,\s*the$`)

// artistKey folds case, accents, "&"/"+"/"and", a leading "The", spaces
// and punctuation, so spellings of one artist share a key.
func artistKey(name string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(name) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	s := strings.ToLower(strings.TrimSpace(b.String()))
	s = strings.NewReplacer("&", " and ", "+", " and ").Replace(s)
	s = leadingThe.ReplaceAllString(s, "")
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
}

func (l *ArtistVariants) Run(ctx context.Context) ([]doctor.Issue, error) {
	groups := make(map[string][]beets.Album)
	for _, a := range l.albums {
		if a.AlbumArtist == vaName {
			continue
		}
		if k := artistKey(a.AlbumArtist); k != "" {
			groups[k] = append(groups[k], a)
		}
	}

	var issues []doctor.Issue
	for _, albums := range groups {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		names := make(map[string]int)
		ids := make(map[string]bool)
		mbNames := make(map[string]int)
		for _, a := range albums {
			names[a.AlbumArtist]++
			ids[a.MBAlbumArtistID] = true
			if mbidPattern.MatchString(a.MBAlbumArtistID) {
				mbNames[a.AlbumArtist]++
			}
		}
		if len(names) == 1 && len(ids) == 1 {
			continue
		}
		want := mostCommon(mbNames)
		for _, a := range albums {
			if a.AlbumArtist == want && mbidPattern.MatchString(a.MBAlbumArtistID) {
				continue
			}
			issues = append(issues, doctor.Issue{
				Path:        a.Path,
				AlbumID:     a.ID,
				Description: fmt.Sprintf("%s — %s: %s", a.AlbumArtist, a.Album, otherSpellings(names, a.AlbumArtist, want)),
				Severity:    doctor.SeverityWarning,
			})
		}
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].Description < issues[j].Description })
	return issues, nil
}

// mostCommon returns the name used most, or "" for no names. Ties go to the
// name that sorts first, so the result does not change between runs.
func mostCommon(counts map[string]int) string {
	best := ""
	for n, c := range counts {
		if c > counts[best] || (c == counts[best] && n < best) {
			best = n
		}
	}
	return best
}

func otherSpellings(names map[string]int, self, want string) string {
	var parts []string
	for n, c := range names {
		if n == self {
			continue
		}
		p := fmt.Sprintf("%q (%d)", n, c)
		if n == want {
			p += " with MusicBrainz ID"
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return "same name also used with another artist ID"
	}
	sort.Strings(parts)
	return "also written as " + strings.Join(parts, ", ")
}
