package beets

import (
	"database/sql"
	"fmt"
	"path/filepath"

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

// UnmarkedAlbums returns the albums in ids that still exist and do not have
// the flexible field set, in the order of ids. 'beet import -L' gives an
// album a new ID when it applies a match, so a missing ID means that album
// was already retagged. The field catches the rare case where SQLite hands
// the old, highest ID to the new album.
func UnmarkedAlbums(dbPath, field string, ids []int) ([]LibraryAlbum, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open beets db: %w", err)
	}
	defer db.Close()

	const query = `
SELECT
    a.id,
    COALESCE(a.albumartist, '') AS albumartist,
    COALESCE(a.album, '')       AS album
FROM albums a
WHERE NOT EXISTS (
    SELECT 1 FROM album_attributes aa WHERE aa.entity_id = a.id AND aa.key = ?
)
`
	rows, err := db.Query(query, field)
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
