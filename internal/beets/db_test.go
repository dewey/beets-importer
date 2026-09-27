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
INSERT INTO albums VALUES (5, 'Kode9', 'Memories of the Future');
INSERT INTO album_attributes VALUES (1, 2, 'retagged', '2026-09-27'), (2, 3, 'other', 'x'), (3, 5, 'retag_skipped', '2026-09-27');
`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	// 2 and 5 are marked and 4 is gone, so only 3 and 1 are left, in list order.
	albums, err := UnmarkedAlbums(dbPath, []string{"retagged", "retag_skipped"}, []int{3, 2, 5, 4, 1})
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

func TestAlbumIDsByPath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "library.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	// Item 3 is stored as NFD ("o" plus a combining mark), item 4 is a singleton.
	_, err = db.ExecContext(t.Context(), `
CREATE TABLE items (id INTEGER PRIMARY KEY, album_id INTEGER, path BLOB);
INSERT INTO items VALUES
    (1, 10, CAST('/lib/Burial/Untrue/01.flac' AS BLOB)),
    (2, 10, CAST('/lib/Burial/Untrue/02.flac' AS BLOB)),
    (3, 20, CAST('/lib/Björk/Post/01.flac' AS BLOB)),
    (4, NULL, CAST('/lib/Non-Album/single.mp3' AS BLOB));
`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	ids, missing, err := AlbumIDsByPath(dbPath, []string{
		"/lib/Björk/Post/01.flac",
		"/lib/Burial/Untrue/02.flac",
		"/lib/Burial/Untrue/01.flac",
		"/lib/Non-Album/single.mp3",
		"/lib/gone.mp3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != 20 || ids[1] != 10 {
		t.Errorf("ids = %v, want [20 10]", ids)
	}
	if len(missing) != 2 {
		t.Errorf("missing = %v, want the singleton and the gone file", missing)
	}
}

func TestItemCounts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "library.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(t.Context(), `
CREATE TABLE items (id INTEGER PRIMARY KEY, album_id INTEGER, path BLOB);
INSERT INTO items VALUES (1, 10, 'a'), (2, 10, 'b'), (3, 20, 'c');
`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	counts, err := ItemCounts(dbPath, []int{10, 20, 30})
	if err != nil {
		t.Fatal(err)
	}
	if len(counts) != 2 || counts[10] != 2 || counts[20] != 1 {
		t.Errorf("counts = %v, want 10:2 20:1", counts)
	}
}

func TestAlbumFolders(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "library.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.ExecContext(t.Context(), `
CREATE TABLE items (id INTEGER PRIMARY KEY, album_id INTEGER, path BLOB);
INSERT INTO items VALUES
    (1, 10, CAST('/lib/Nas/Illmatic/CD1/01.flac' AS BLOB)),
    (2, 10, CAST('/lib/Nas/Illmatic/CD1/02.flac' AS BLOB)),
    (3, 10, CAST('/lib/Nas/Illmatic/CD2/01.flac' AS BLOB)),
    (4, 20, CAST('/lib/Burial/Untrue/01.flac' AS BLOB));
`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	folders, err := AlbumFolders(dbPath, []int{10})
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) != 1 || len(folders[10]) != 2 {
		t.Errorf("folders = %v, want two folders for album 10 only", folders)
	}
}
