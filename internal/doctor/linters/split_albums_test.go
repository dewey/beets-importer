package linters_test

import (
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

func TestSplitAlbums(t *testing.T) {
	albums := []beets.Album{
		{ID: 1, Path: "/music/ok", AlbumArtist: "Burial", Album: "Untrue"},
		{ID: 2, Path: "/music/mixed-id", AlbumArtist: "Amy Winehouse", Album: "Frank"},
		{ID: 3, Path: "/music/case", AlbumArtist: "Lil Wayne", Album: "Tha Carter III"},
		{ID: 4, Path: "/music/empty-artist", AlbumArtist: "Various Artists", Album: "Jazz for Dinner"},
	}
	items := []beets.Item{
		{AlbumID: 1, AlbumArtist: "Burial", Album: "Untrue"},
		{AlbumID: 1, AlbumArtist: "Burial", Album: "Untrue"},
		{AlbumID: 2, AlbumArtist: "Amy Winehouse", Album: "Frank"},
		{AlbumID: 2, AlbumArtist: "Amy Winehouse", Album: "Frank", MBAlbumID: "13a69b29-09b2-4032-8358-6bc0f05d38c3"},
		{AlbumID: 3, AlbumArtist: "Lil Wayne", Album: "Tha Carter III"},
		{AlbumID: 3, AlbumArtist: "Lil Wayne", Album: "Tha Carter Iii"},
		{AlbumID: 4, AlbumArtist: "", Album: "Jazz for Dinner"},
		{AlbumID: 0, AlbumArtist: "singleton"},
	}
	issues, err := linters.NewSplitAlbums(albums, items).Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	assertIssues(t, issues, []string{"/music/mixed-id", "/music/case", "/music/empty-artist"})
}
