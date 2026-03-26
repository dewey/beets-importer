package compare

import (
	"fmt"
	"strings"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/source"
)

// formatRank returns a quality rank for an audio format string.
// Higher = better quality.
func formatRank(f string) int {
	switch strings.ToUpper(f) {
	case "FLAC", "ALAC", "WAV", "AIFF", "APE":
		return 3
	case "MP3", "AAC", "OGG", "OPUS":
		return 2
	default:
		return 1
	}
}

// Candidate represents a source→library pair that is a genuine upgrade opportunity.
type Candidate struct {
	Source      source.Album
	Library     beets.Album
	Score       float64
	Reasons     []string
	YearMatches bool // true when both source and library have a known year and they match
}

// Evaluate checks whether the source album is an upgrade over the library album.
// Returns a Candidate and true if there is at least one upgrade reason,
// otherwise returns zero value and false.
// When requireYearMatch is true, pairs where both sides have a known year that
// differs are rejected outright regardless of other upgrade criteria.
func Evaluate(src source.Album, lib beets.Album, score float64, minBitrateDelta int, requireYearMatch bool) (Candidate, bool) {
	yearMatches := src.Year > 0 && lib.Year > 0 && src.Year == lib.Year
	if requireYearMatch && src.Year > 0 && lib.Year > 0 && !yearMatches {
		return Candidate{}, false
	}

	var reasons []string

	srcRank := formatRank(src.Format)
	libRank := formatRank(lib.Format)

	if srcRank > libRank {
		srcDesc := strings.ToUpper(src.Format)
		libDesc := strings.ToUpper(lib.Format)
		// Show bitrate for lossy formats; lossless bitrate is content-dependent and not meaningful.
		if srcRank < 3 && src.AvgBitrate > 0 {
			srcDesc = fmt.Sprintf("%s (%dkbps)", srcDesc, src.AvgBitrate/1000)
		}
		if libRank < 3 && lib.AvgBitrate > 0 {
			libDesc = fmt.Sprintf("%s (%dkbps)", libDesc, lib.AvgBitrate/1000)
		}
		reasons = append(reasons, fmt.Sprintf("%s replaces %s", srcDesc, libDesc))
	} else if srcRank == libRank && src.AvgBitrate > 0 && lib.AvgBitrate > 0 {
		// Compare bitrates only when formats are equivalent quality tier
		srcKbps := src.AvgBitrate / 1000
		libKbps := lib.AvgBitrate / 1000
		delta := srcKbps - libKbps
		if delta >= minBitrateDelta {
			reasons = append(reasons, fmt.Sprintf(
				"%dkbps replaces %dkbps",
				srcKbps, libKbps,
			))
		}
	}

	if len(reasons) == 0 {
		return Candidate{}, false
	}
	return Candidate{
		Source:      src,
		Library:     lib,
		Score:       score,
		Reasons:     reasons,
		YearMatches: yearMatches,
	}, true
}
