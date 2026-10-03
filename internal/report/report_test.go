package report

import (
	"bytes"
	"database/sql"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const mbid = "11111111-1111-1111-1111-111111111111"

type fakeDisk struct {
	covers map[string]string
}

func (f fakeDisk) Size(path string) (int64, error) {
	if strings.HasSuffix(path, "2.mp3") {
		return 0, nil
	}
	return 1000, nil
}

func (f fakeDisk) CoverFile(dir string) (string, error) { return f.covers[dir], nil }

func fixture(t *testing.T, artpath string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lib.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE albums (id INTEGER PRIMARY KEY, artpath BLOB, added REAL, albumartist TEXT, album TEXT, genre TEXT, year INTEGER, mb_albumid TEXT, discogs_albumid INTEGER, albumtype TEXT, label TEXT)`,
		`CREATE TABLE items (id INTEGER PRIMARY KEY, path BLOB, album_id INTEGER, title TEXT, artist TEXT, track INTEGER, format TEXT, bitrate INTEGER, bitdepth INTEGER, samplerate INTEGER, length REAL,
			mb_trackid TEXT, mb_albumid TEXT, mb_artistid TEXT, acoustid_id TEXT, isrc TEXT, rg_track_gain REAL, rg_album_gain REAL, lyrics TEXT, bpm INTEGER, initial_key TEXT, label TEXT, composer TEXT, media TEXT, comments TEXT)`,
		`CREATE TABLE item_attributes (id INTEGER PRIMARY KEY, entity_id INTEGER, key TEXT, value TEXT)`,
		`CREATE TABLE album_attributes (id INTEGER PRIMARY KEY, entity_id INTEGER, key TEXT, value TEXT)`,
		`INSERT INTO album_attributes (entity_id, key, value) VALUES (1, 'retagged', '2026-10-01'), (3, 'retagged', '2026-10-02')`,
		// 1: FLAC, complete, hi-res, has art. 2: MP3 with gaps, file missing. 3: ALAC, year as full date.
		// 4: mixed, cover in folder. 5: ignored. 6: same artist and name as 1, other case.
		`INSERT INTO albums (id, artpath, added, albumartist, album, genre, year, mb_albumid, discogs_albumid, albumtype, label) VALUES
			(1, '` + artpath + `', 1735689600, 'A', 'One', 'Rock', 2001, '` + mbid + `', 0, 'album', 'L1'),
			(2, NULL, 1735689600, 'B', '', '', 0, '', 0, '', ''),
			(3, NULL, 1738368000, 'C', 'Three', 'Pop', 20161101, '', 5, 'Album', ''),
			(4, NULL, 1738368000, 'D', 'Four', 'Pop', 1999, '', 5, '', ''),
			(5, NULL, 1738368000, 'E', 'Skip Me', 'Pop', 1999, '', 5, '', ''),
			(6, NULL, 1735689600, 'A', 'one', 'Rock', 2001, '', 0, '', '')`,
		`INSERT INTO items (id, path, album_id, title, artist, track, format, bitrate, bitdepth, samplerate, length) VALUES
			(1, '/lib/a/1.flac', 1, 't', 'A', 1, 'FLAC', 900000, 24, 96000, 60),
			(2, '/lib/b/2.mp3', 2, '', 'B', 0, 'MP3', 192000, 0, 0, 30),
			(3, '/lib/c/3.m4a', 3, 't', 'C', 1, 'ALAC', 800000, 16, 44100, 30),
			(4, '/lib/d/4.flac', 4, 't', 'D', 1, 'FLAC', 900000, 16, 44100, 30),
			(5, '/lib/d/5.mp3', 4, 't', 'D', 2, 'MP3', 320000, 0, 0, 30),
			(6, '/lib/e/6.mp3', 5, 't', 'E', 1, 'MP3', 128000, 0, 0, 30),
			(7, '/lib/s/7.flac', NULL, 'single', 'S', 1, 'FLAC', 900000, 16, 44100, 30),
			(8, '/lib/a2/8.flac', 6, 't', 'A', 1, 'FLAC', 900000, 16, 44100, 30)`,
	}
	for _, s := range stmts {
		if _, err = db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func writeCover(t *testing.T) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 600, 400))
	for x := 0; x < 600; x++ {
		for y := 0; y < 400; y++ {
			img.Set(x, y, color.RGBA{200, 50, 50, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCollect(t *testing.T) {
	ignored := func(name string) bool { return name == "Skip Me" }
	disk := fakeDisk{covers: map[string]string{"/lib/d": "cover.jpg"}}
	d, err := Collect(fixture(t, writeCover(t)), ignored, time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC), disk)
	if err != nil {
		t.Fatal(err)
	}

	if d.Totals.Tracks != 7 || d.Totals.Albums != 5 || d.Totals.Singletons != 1 || d.Totals.HiRes != 1 {
		t.Errorf("totals: %+v", d.Totals)
	}
	if d.Totals.Bytes != 6000 || d.Totals.MissingFiles != 1 {
		t.Errorf("sizes: %+v", d.Totals)
	}
	want := map[string]int{ClassLossless: 3, ClassLossy: 1, ClassMixed: 1}
	for class, n := range want {
		if d.Progress.Albums[class] != n {
			t.Errorf("progress %s = %d, want %d", class, d.Progress.Albums[class], n)
		}
	}
	if d.Years[len(d.Years)-1].Name != "2016" {
		t.Errorf("full date year not truncated: %+v", d.Years)
	}
	if g := d.Growth; len(g) != 2 || g[1].Lossless != 3000 || g[1].Lossy != 2000 {
		t.Errorf("growth: %+v", g)
	}
	if got := d.AlbumSizes[0]; got.Name != "1" || got.Count != 4 {
		t.Errorf("album sizes: %+v", d.AlbumSizes)
	}
	if len(d.Internal.Attributes) != 1 || d.Internal.Attributes[0].Count != 2 || d.Internal.Fields[0].Missing != 8 || d.Internal.DuplicateAlbums != 1 {
		t.Errorf("internal: %+v", d.Internal)
	}
	if len(d.AddedDays) != 2 || d.AddedDays[0].Count != 3 {
		t.Errorf("added days: %+v", d.AddedDays)
	}
	if len(d.AlbumTypes) != 2 || d.AlbumTypes[0].Name != "(unset)" || len(d.Labels) != 1 {
		t.Errorf("types %+v labels %+v", d.AlbumTypes, d.Labels)
	}
	if len(d.LossyArtists) != 2 || d.LossyArtists[0].Artist != "B" || d.LossyArtists[1].Total != 1 {
		t.Errorf("lossy artists: %+v", d.LossyArtists)
	}

	gaps := map[string]Gap{}
	for _, g := range d.Gaps {
		gaps[g.Key] = g
	}
	wantGaps := map[string]int{
		"lossy": 1, "mixed": 1, "duplicates": 2, "missing": 1, "noname": 1, "noyear": 1, "noart": 3, "folderart": 1,
		"nogenre": 1, "nosource": 2, "notitle": 1, "notrack": 1, "singleton": 1,
	}
	for k, n := range wantGaps {
		if gaps[k].Count != n {
			t.Errorf("gap %s = %d, want %d", k, gaps[k].Count, n)
		}
	}
	if u := gaps["duplicates"].Rows[0].URL; u != "https://musicbrainz.org/release/"+mbid {
		t.Errorf("duplicate url: %q", u)
	}

	if len(d.Recent) != 5 {
		t.Fatalf("recent: %+v", d.Recent)
	}
	var withThumb int
	for _, r := range d.Recent {
		if strings.HasPrefix(r.Thumb, "data:image/jpeg;base64,") && strings.HasPrefix(r.Cover, "file:///") {
			withThumb++
		}
	}
	if withThumb != 1 {
		t.Errorf("recent albums with thumbnail: %d", withThumb)
	}

	if got := d.Snapshot(); got.Day != "2026-10-03" || got.LossyAlbums != 2 || got.LosslessAlbums != 3 || got.LosslessTracks != 3 || got.Bytes != 6000 {
		t.Errorf("snapshot: %+v", got)
	}
}

func TestRender(t *testing.T) {
	d, err := Collect(fixture(t, ""), nil, time.Now(), fakeDisk{})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err = Render(&buf, d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"albums":6`) {
		t.Error("data not embedded in page")
	}
}
