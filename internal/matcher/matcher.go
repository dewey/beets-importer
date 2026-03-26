package matcher

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/agnivade/levenshtein"
	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/source"
)

// Match pairs a source album with its best-matching beets library album.
type Match struct {
	Source  source.Album
	Library beets.Album
	Score   float64 // 0..1, higher is more confident
}

// punctRe strips characters that are neither Unicode letters/digits nor whitespace.
// Using \p{L}\p{N} (not \w) so that non-Latin scripts (CJK, Hebrew, Arabic, etc.)
// are preserved rather than being incorrectly stripped to empty strings.
var punctRe = regexp.MustCompile(`[^\p{L}\p{N}\s]`)

// normalize lowercases, strips punctuation, and collapses whitespace.
func normalize(s string) string {
	s = strings.ToLower(s)
	s = punctRe.ReplaceAllString(s, " ")
	var b strings.Builder
	prevSpace := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteRune(' ')
			}
			prevSpace = true
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}

// similarity returns a 0..1 score: 1 means identical, 0 means maximally different.
func similarity(a, b string) float64 {
	a = normalize(a)
	b = normalize(b)
	if a == "" && b == "" {
		// Both inputs were empty or all-punctuation — no usable text to compare.
		return 0
	}
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	dist := levenshtein.ComputeDistance(a, b)
	maxLen := len([]rune(a))
	if lb := len([]rune(b)); lb > maxLen {
		maxLen = lb
	}
	return 1 - float64(dist)/float64(maxLen)
}

// FindMatches matches each source album to the best library album with score >= threshold.
// Each library album is matched at most once (greedy: highest score wins).
func FindMatches(sourceAlbums []source.Album, libraryAlbums []beets.Album, threshold float64) []Match {
	// Pre-normalize library entries
	type normLib struct {
		album      beets.Album
		normArtist string
		normAlbum  string
	}
	norm := make([]normLib, len(libraryAlbums))
	for i, a := range libraryAlbums {
		norm[i] = normLib{
			album:      a,
			normArtist: normalize(a.AlbumArtist),
			normAlbum:  normalize(a.Album),
		}
	}

	used := make([]bool, len(libraryAlbums))

	var matches []Match
	for _, src := range sourceAlbums {
		nSrcArtist := normalize(src.Artist)
		nSrcAlbum := normalize(src.Album)

		bestScore := -1.0
		bestIdx := -1

		for i, lib := range norm {
			if used[i] {
				continue
			}
			artistSim := similarity(nSrcArtist, lib.normArtist)
			albumSim := similarity(nSrcAlbum, lib.normAlbum)

			// yearSim is 1 when both sides have a known year and it matches,
			// 0 when both sides have a known year and it doesn't match,
			// and contributes nothing (weight collapses back to name-only) when
			// either year is unknown.
			// Pairs where both years are known and differ by more than 3 are
			// skipped entirely — they are almost certainly different editions or
			// different albums.
			yearKnown := src.Year > 0 && lib.album.Year > 0
			if yearKnown {
				diff := src.Year - lib.album.Year
				if diff < 0 {
					diff = -diff
				}
				if diff > 3 {
					continue
				}
			}
			var yearSim float64
			if yearKnown && src.Year == lib.album.Year {
				yearSim = 1
			}

			// Weights: album 0.55, artist 0.35, year 0.10 (when year is known on both sides).
			// When year is unknown the 0.10 share is redistributed proportionally to names.
			// When artist is missing, artist's share goes entirely to album.
			var score float64
			if yearKnown {
				if nSrcArtist == "" {
					score = 0.9*albumSim + 0.1*yearSim
				} else {
					score = 0.35*artistSim + 0.55*albumSim + 0.1*yearSim
				}
			} else {
				if nSrcArtist == "" {
					score = albumSim
				} else {
					score = 0.4*artistSim + 0.6*albumSim
				}
			}

			if score > bestScore {
				bestScore = score
				bestIdx = i
			}
		}

		if bestIdx >= 0 && bestScore >= threshold {
			used[bestIdx] = true
			matches = append(matches, Match{
				Source:  src,
				Library: norm[bestIdx].album,
				Score:   bestScore,
			})
		}
	}
	return matches
}
