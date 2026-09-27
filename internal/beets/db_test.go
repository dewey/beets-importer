package beets

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestUnmarkedAlbums(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "library.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(t.Context(), `
CREATE TABLE albums (id INTEGER PRIMARY KEY, albumartist TEXT, album TEXT);
CREATE TABLE album_attributes (id INTEGER PRIMARY KEY, entity_id INTEGER, key TEXT, value TEXT);
INSERT INTO albums VALUES (1, 'Burial', 'Untrue'), (2, 'Kode9', 'Nothing'), (3, 'Lady GaGa', 'The Fame');
INSERT INTO album_attributes VALUES (1, 2, 'retagged', '2026-09-27'), (2, 3, 'other', 'x');
`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	// 2 is marked and 4 is gone, so only 3 and 1 are left, in list order.
	albums, err := UnmarkedAlbums(dbPath, "retagged", []int{3, 2, 4, 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(albums) != 2 || albums[0].ID != 3 || albums[1].ID != 1 {
		t.Fatalf("albums = %+v, want IDs 3, 1", albums)
	}
	if albums[0].AlbumArtist != "Lady GaGa" || albums[0].Album != "The Fame" {
		t.Errorf("albums[0] = %+v", albums[0])
	}
}
