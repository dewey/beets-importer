package beets

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/text/unicode/norm"
	_ "modernc.org/sqlite"
)

// Album represents an album in the beets library, aggregated from albums + items tables.
type Album struct {
	ID              int
	AlbumArtist     string
	MBAlbumArtistID string // can also hold a Discogs ID or Bandcamp URL, set by those plugins
	Album           string
	Year            int
	Format          string // dominant format across tracks, e.g. "MP3", "FLAC"
	AvgBitrate      int    // average bitrate in bps across tracks
	TrackCount      int
	HasArtwork      bool   // true if albums.artpath is non-empty
	Path            string // directory containing the album's tracks
}

// Item represents a single track in the beets library.
type Item struct {
	ID          int
	AlbumID     int
	Artist      string
	AlbumArtist string
	Album       string
	MBAlbumID   string
	Title       string
	Format      string
	Bitrate     int // in bps
	Path        string
}

// LoadItems opens the beets SQLite database and returns all tracks.
func LoadItems(dbPath string) ([]Item, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open beets db: %w", err)
	}
	defer db.Close()

	const query = `
SELECT
    id,
    COALESCE(album_id, 0)   AS album_id,
    COALESCE(artist, '')      AS artist,
    COALESCE(albumartist, '') AS albumartist,
    COALESCE(album, '')       AS album,
    COALESCE(mb_albumid, '')  AS mb_albumid,
    COALESCE(title, '')     AS title,
    COALESCE(format, '')    AS format,
    COALESCE(bitrate, 0)    AS bitrate,
    COALESCE(path, '')      AS path
FROM items
`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(
			&it.ID, &it.AlbumID, &it.Artist, &it.AlbumArtist, &it.Album,
			&it.MBAlbumID, &it.Title, &it.Format, &it.Bitrate, &it.Path,
		); err != nil {
			return nil, fmt.Errorf("scan item row: %w", err)
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// LoadAlbums opens the beets SQLite database and returns all albums with aggregated track info.
func LoadAlbums(dbPath string) ([]Album, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open beets db: %w", err)
	}
	defer db.Close()

	// We pick the format from the most common format among tracks (via a subquery).
	// AVG(bitrate) gives average in bps.
	const query = `
SELECT
    a.id,
    COALESCE(a.albumartist, '') AS albumartist,
    COALESCE(a.mb_albumartistid, '') AS mb_albumartistid,
    COALESCE(a.album, '')       AS album,
    COALESCE(a.year, 0)         AS year,
    COALESCE(a.artpath, '')     AS artpath,
    COUNT(i.id)                 AS track_count,
    CAST(AVG(i.bitrate) AS INTEGER) AS avg_bitrate,
    COALESCE(MIN(i.path), '')   AS sample_path,
    COALESCE((
        SELECT i2.format
        FROM items i2
        WHERE i2.album_id = a.id AND i2.format != ''
        GROUP BY i2.format
        ORDER BY COUNT(*) DESC
        LIMIT 1
    ), '') AS dominant_format
FROM albums a
JOIN items i ON i.album_id = a.id
GROUP BY a.id
`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query albums: %w", err)
	}
	defer rows.Close()

	var albums []Album
	for rows.Next() {
		var a Album
		var artpath, samplePath string
		if err := rows.Scan(
			&a.ID, &a.AlbumArtist, &a.MBAlbumArtistID, &a.Album, &a.Year,
			&artpath, &a.TrackCount, &a.AvgBitrate, &samplePath, &a.Format,
		); err != nil {
			return nil, fmt.Errorf("scan album row: %w", err)
		}
		a.HasArtwork = artpath != ""
		if samplePath != "" {
			a.Path = filepath.Dir(samplePath)
		}
		// Beets sometimes stores a full date as YYYYMMDD in the year column when
		// audio files contain a compact (no-hyphen) DATE tag. This is a bug in
		// beetbox/mediafile — the date parser splits on "-" or "/" only, so
		// "20161101" is never decomposed and lands wholesale in the year field.
		// See: https://github.com/beetbox/mediafile/pull/100
		// Workaround: truncate to just the year until the upstream fix ships.
		if a.Year > 9999 {
			a.Year = a.Year / 10000
		}
		albums = append(albums, a)
	}
	return albums, rows.Err()
}

// LibraryAlbum is an album picked by ID, for display while retagging.
type LibraryAlbum struct {
	ID          int
	AlbumArtist string
	Album       string
}

// UnmarkedAlbums returns the albums in ids that still exist and have none of
// the flexible fields set, in the order of ids. 'beet import -L' gives an
// album a new ID when it applies a match, so a missing ID means that album
// was already retagged. The field catches the rare case where SQLite hands
// the old, highest ID to the new album.
func UnmarkedAlbums(dbPath string, fields []string, ids []int) ([]LibraryAlbum, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open beets db: %w", err)
	}
	defer db.Close()

	query := `
SELECT
    a.id,
    COALESCE(a.albumartist, '') AS albumartist,
    COALESCE(a.album, '')       AS album
FROM albums a
WHERE NOT EXISTS (
    SELECT 1 FROM album_attributes aa WHERE aa.entity_id = a.id AND aa.key IN (` + placeholders(len(fields)) + `)
)
`
	rows, err := db.Query(query, anys(fields)...)
	if err != nil {
		return nil, fmt.Errorf("query albums: %w", err)
	}
	defer rows.Close()

	byID := make(map[int]LibraryAlbum)
	for rows.Next() {
		var a LibraryAlbum
		if err := rows.Scan(&a.ID, &a.AlbumArtist, &a.Album); err != nil {
			return nil, fmt.Errorf("scan album row: %w", err)
		}
		byID[a.ID] = a
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var albums []LibraryAlbum
	for _, id := range ids {
		if a, ok := byID[id]; ok {
			albums = append(albums, a)
		}
	}
	return albums, nil
}

// AlbumIDsByPath returns the distinct album IDs of the tracks at paths, in the
// order they first appear. Paths with no album track in the library are
// returned as missing. Paths are compared as NFC, because macOS may store
// them as NFD.
func AlbumIDsByPath(dbPath string, paths []string) (ids []int, missing []string, err error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, nil, fmt.Errorf("open beets db: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT CAST(i.path AS TEXT), i.album_id FROM items i WHERE i.album_id IS NOT NULL`)
	if err != nil {
		return nil, nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()

	albumOf := make(map[string]int)
	for rows.Next() {
		var p string
		var id int
		if err := rows.Scan(&p, &id); err != nil {
			return nil, nil, fmt.Errorf("scan item row: %w", err)
		}
		albumOf[norm.NFC.String(p)] = id
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	seen := make(map[int]bool)
	for _, p := range paths {
		id, ok := albumOf[norm.NFC.String(p)]
		if !ok {
			missing = append(missing, p)
			continue
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, missing, nil
}

// MarkedAlbums returns the IDs of albums that have any of the flexible fields set.
func MarkedAlbums(dbPath string, fields []string) (map[int]bool, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open beets db: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT aa.entity_id FROM album_attributes aa WHERE aa.key IN (`+placeholders(len(fields))+`)`, anys(fields)...)
	if err != nil {
		return nil, fmt.Errorf("query album attributes: %w", err)
	}
	defer rows.Close()

	ids := make(map[int]bool)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan album attribute: %w", err)
		}
		ids[id] = true
	}
	return ids, rows.Err()
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func anys(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// ItemCounts returns how many tracks each of the albums has. Albums without
// tracks are left out.
func ItemCounts(dbPath string, albumIDs []int) (map[int]int, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open beets db: %w", err)
	}
	defer db.Close()

	args := make([]any, len(albumIDs))
	for i, id := range albumIDs {
		args[i] = id
	}
	rows, err := db.Query(`SELECT i.album_id, COUNT(*) FROM items i WHERE i.album_id IN (`+placeholders(len(albumIDs))+`) GROUP BY i.album_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()

	counts := make(map[int]int)
	for rows.Next() {
		var id, n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan item count: %w", err)
		}
		counts[id] = n
	}
	return counts, rows.Err()
}

// AlbumFolders returns the folders that hold each album's tracks, cleaned
// and in NFC form. A multi-disc album can have one folder per disc.
func AlbumFolders(dbPath string, albumIDs []int) (map[int][]string, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open beets db: %w", err)
	}
	defer db.Close()

	args := make([]any, len(albumIDs))
	for i, id := range albumIDs {
		args[i] = id
	}
	rows, err := db.Query(`SELECT i.album_id, CAST(i.path AS TEXT) FROM items i WHERE i.album_id IN (`+placeholders(len(albumIDs))+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()

	seen := make(map[int]map[string]bool)
	folders := make(map[int][]string)
	for rows.Next() {
		var id int
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			return nil, fmt.Errorf("scan item row: %w", err)
		}
		dir := norm.NFC.String(filepath.Dir(p))
		if seen[id] == nil {
			seen[id] = make(map[string]bool)
		}
		if !seen[id][dir] {
			seen[id][dir] = true
			folders[id] = append(folders[id], dir)
		}
	}
	return folders, rows.Err()
}
