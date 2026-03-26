package cmd

import (
	"fmt"
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/compare"
	"github.com/dewey/beets-importer/internal/source"
)

// TestPickerLayout prints sample picker rows to stdout so you can visually
// inspect the two-line table layout. Run with:
//
//	go test -v -run TestPickerLayout ./cmd/
func TestPickerLayout(t *testing.T) {
	candidates := []compare.Candidate{
		{
			// All fields match, lossless upgrade
			Source:  source.Album{Artist: "Blumentopf", Album: "Kein Zufall", Year: 1999, Format: "FLAC", TrackCount: 16},
			Library: beets.Album{AlbumArtist: "Blumentopf", Album: "Kein Zufall", Year: 1999, Format: "MP3", AvgBitrate: 192000, TrackCount: 16},
			Score:   0.90,
			Reasons: []string{"FLAC replaces MP3 (192kbps)"},
		},
		{
			// Perfect match
			Source:  source.Album{Artist: "Optimus Rhyme", Album: "School the World", Year: 2006, Format: "FLAC", TrackCount: 12},
			Library: beets.Album{AlbumArtist: "Optimus Rhyme", Album: "School the World", Year: 2006, Format: "MP3", AvgBitrate: 320000, TrackCount: 12},
			Score:   1.00,
			Reasons: []string{"FLAC replaces MP3 (320kbps)"},
		},
		{
			// Track count mismatch (EP vs album) — should highlight in orange
			Source:  source.Album{Artist: "Curren$y", Album: "Pilot Talk II", Year: 2010, Format: "FLAC", TrackCount: 6},
			Library: beets.Album{AlbumArtist: "Curren$y", Album: "Pilot Talk III", Year: 2015, Format: "MP3", AvgBitrate: 256000, TrackCount: 14},
			Score:   0.86,
			Reasons: []string{"FLAC replaces MP3 (256kbps)"},
		},
		{
			// Same format, higher bitrate
			Source:  source.Album{Artist: "Alex Calder", Album: "Time", Year: 2013, Format: "MP3", AvgBitrate: 385000, TrackCount: 5},
			Library: beets.Album{AlbumArtist: "Alex Calder", Album: "Time", Year: 2013, Format: "MP3", AvgBitrate: 320000, TrackCount: 5},
			Score:   0.76,
			Reasons: []string{"385kbps replaces 320kbps"},
		},
		{
			// Unknown year on source side
			Source:  source.Album{Artist: "Lurka", Album: "Return / Stabiliser", Year: 0, Format: "FLAC", TrackCount: 2},
			Library: beets.Album{AlbumArtist: "Lurka", Album: "Return / Stabiliser", Year: 2019, Format: "MP3", AvgBitrate: 320000, TrackCount: 2},
			Score:   0.90,
			Reasons: []string{"FLAC replaces MP3 (320kbps)"},
		},
		{
			// Long names to exercise truncation
			Source:  source.Album{Artist: "Timbaland & Magoo", Album: "Welcome To Our World (Deluxe Edition)", Year: 1997, Format: "FLAC", TrackCount: 18},
			Library: beets.Album{AlbumArtist: "Timbaland & Magoo", Album: "Welcome to Our World", Year: 1997, Format: "MP3", AvgBitrate: 128000, TrackCount: 15},
			Score:   0.90,
			Reasons: []string{"FLAC replaces MP3 (128kbps)"},
		},
	}

	// Simulate the picker separator and header.
	sep := "──────────────────────────────────────────────────────────────────────────────────────────"
	fmt.Println("Select albums to import  SPACE toggle · CTRL+A all · ENTER confirm · ESC cancel")
	fmt.Println(sep)
	for i, c := range candidates {
		item := buildPickerItem(c)
		cursor := "  "
		if i == 0 {
			cursor = "▶ "
		}
		check := "[ ]"
		fmt.Printf("%s%s %s\n", cursor, check, item.Line1)
		fmt.Printf("      %s\n", item.Line2)
	}
	fmt.Println(sep)
}
