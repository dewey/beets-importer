package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store is a small SQLite database for data that has no home in the beets
// database, like report snapshots. It lives next to the config file.
type Store struct {
	db *sql.DB
}

// Snapshot is the library state on one day.
type Snapshot struct {
	Day            string `json:"day"` // YYYY-MM-DD
	Tracks         int    `json:"tracks"`
	Albums         int    `json:"albums"`
	LosslessAlbums int    `json:"losslessAlbums"`
	LosslessTracks int    `json:"losslessTracks"`
	Bytes          int64  `json:"bytes"`       // estimated size of all tracks
	LossyAlbums    int    `json:"lossyAlbums"` // lossy and mixed albums
}

const schema = `
CREATE TABLE IF NOT EXISTS snapshots (
    day            TEXT PRIMARY KEY,
    tracks         INTEGER NOT NULL,
    albums         INTEGER NOT NULL,
    lossless_albums INTEGER NOT NULL,
    lossless_tracks INTEGER NOT NULL,
    bytes           INTEGER NOT NULL,
    lossy_albums   INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS file_sizes (
    path  TEXT PRIMARY KEY,
    bytes INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS dir_covers (
    dir  TEXT PRIMARY KEY,
    file TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
)`

// Open opens the store at path and creates it when missing.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	if _, err = db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create store schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// SaveSnapshot stores the snapshot. A second one on the same day replaces the first.
func (s *Store) SaveSnapshot(sn Snapshot) error {
	_, err := s.db.Exec(`
INSERT OR REPLACE INTO snapshots (day, tracks, albums, lossless_albums, lossless_tracks, bytes, lossy_albums)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sn.Day, sn.Tracks, sn.Albums, sn.LosslessAlbums, sn.LosslessTracks, sn.Bytes, sn.LossyAlbums)
	if err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}
	return nil
}

// Snapshots returns all snapshots, oldest first.
func (s *Store) Snapshots() ([]Snapshot, error) {
	rows, err := s.db.Query(`
SELECT s.day, s.tracks, s.albums, s.lossless_albums, s.lossless_tracks, s.bytes, s.lossy_albums
FROM snapshots s ORDER BY s.day`)
	if err != nil {
		return nil, fmt.Errorf("query snapshots: %w", err)
	}
	defer rows.Close()

	var out []Snapshot
	for rows.Next() {
		var sn Snapshot
		if err = rows.Scan(&sn.Day, &sn.Tracks, &sn.Albums, &sn.LosslessAlbums, &sn.LosslessTracks, &sn.Bytes, &sn.LossyAlbums); err != nil {
			return nil, fmt.Errorf("scan snapshot: %w", err)
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}
