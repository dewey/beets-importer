package linters_test

import (
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

func TestArtistVariants(t *testing.T) {
	const gaga = "650e7db6-b795-4eb5-a702-5ea2fc46c848"
	albums := []beets.Album{
		{Path: "/music/gaga-mb", AlbumArtist: "Lady Gaga", MBAlbumArtistID: gaga},
		{Path: "/music/gaga-case", AlbumArtist: "Lady GaGa"},
		{Path: "/music/gaga-no-id", AlbumArtist: "Lady Gaga"},
		{Path: "/music/bloodhound", AlbumArtist: "Bloodhound Gang"},
		{Path: "/music/the-bloodhound", AlbumArtist: "The Bloodhound Gang"},
		{Path: "/music/eyedea-amp", AlbumArtist: "Eyedea & Abilities"},
		{Path: "/music/eyedea-and", AlbumArtist: "Eyedea And Abilities"},
		{Path: "/music/doom-mb", AlbumArtist: "MF DOOM", MBAlbumArtistID: "188711ed-c99b-439c-844a-ca831f63a727"},
		{Path: "/music/doom-bandcamp", AlbumArtist: "MF DOOM", MBAlbumArtistID: "https://mfdoom.bandcamp.com"},
		{Path: "/music/burial-1", AlbumArtist: "Burial"},
		{Path: "/music/burial-2", AlbumArtist: "Burial"},
		{Path: "/music/va-mb", AlbumArtist: "Various Artists", MBAlbumArtistID: "89ad4ac3-39f7-470e-963a-56509c546377"},
		{Path: "/music/va-no-id", AlbumArtist: "Various Artists"},
	}
	issues, err := linters.NewArtistVariants(albums).Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	assertIssues(t, issues, []string{
		"/music/gaga-case", "/music/gaga-no-id",
		"/music/bloodhound", "/music/the-bloodhound",
		"/music/eyedea-amp", "/music/eyedea-and",
		"/music/doom-bandcamp",
	})
}
