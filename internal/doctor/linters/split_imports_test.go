package linters_test

import (
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

func TestSplitImportGroups(t *testing.T) {
	albums := []beets.Album{
		// Split by featured artist, all in one folder.
		{ID: 1468, AlbumArtist: "Doppeltes Risiko", Album: "Doppeltes Risiko", TrackCount: 2, Path: "/lib/dr"},
		{ID: 1469, AlbumArtist: "Doppeltes Risiko", Album: "Doppeltes Risiko", TrackCount: 1, Path: "/lib/dr"},
		{ID: 1470, AlbumArtist: "Doppeltes Risiko", Album: "Doppeltes Risiko", TrackCount: 1, Path: "/lib/dr"},
		// A compilation imported track by track, one folder per track.
		{ID: 565, AlbumArtist: "Various Artists", Album: "FM4 Soundselection Vol. 21", TrackCount: 1, Path: "/lib/fm4 [565]"},
		{ID: 575, AlbumArtist: "Various Artists", Album: "fm4 soundselection vol. 21", TrackCount: 1, Path: "/lib/fm4 [575]"},
		// Two copies of disc 2 in one folder: same titles, left alone.
		{ID: 3865, AlbumArtist: "The Libertines", Album: "Babyshambles Sessions", TrackCount: 2, Path: "/lib/bs"},
		{ID: 5942, AlbumArtist: "The Libertines", Album: "Babyshambles Sessions", TrackCount: 2, Path: "/lib/bs"},
		// Two editions in different folders: left alone.
		{ID: 20, AlbumArtist: "Kode9", Album: "Nothing", TrackCount: 13, Path: "/lib/k9 a"},
		{ID: 21, AlbumArtist: "Kode9", Album: "Nothing", TrackCount: 12, Path: "/lib/k9 b"},
	}
	items := []beets.Item{
		{AlbumID: 1468, Title: "Was!"}, {AlbumID: 1468, Title: "Leise…"},
		{AlbumID: 1469, Title: "Stress Im Club"}, {AlbumID: 1470, Title: "Hektik"},
		{AlbumID: 565, Title: "a"}, {AlbumID: 575, Title: "b"},
		{AlbumID: 3865, Title: "albion1"}, {AlbumID: 3865, Title: "france 2"},
		{AlbumID: 5942, Title: "albion1"}, {AlbumID: 5942, Title: "France 2"},
	}
	groups := linters.SplitImportGroups(albums, items)
	if len(groups) != 2 {
		t.Fatalf("groups = %+v, want Doppeltes Risiko and FM4", groups)
	}
	if g := groups[0]; len(g) != 3 || g[0].ID != 1468 {
		t.Errorf("groups[0] = %+v, want 1468, 1469, 1470", g)
	}
	if g := groups[1]; len(g) != 2 || g[0].ID != 565 {
		t.Errorf("groups[1] = %+v, want 565, 575", g)
	}

	issues, err := linters.NewSplitImports(albums, items).Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 || issues[0].Description != "Doppeltes Risiko — Doppeltes Risiko: 4 tracks split into 3 albums" {
		t.Errorf("issues = %+v", issues)
	}
}
