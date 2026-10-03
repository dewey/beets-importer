package report

import (
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dewey/beets-importer/internal/store"
	_ "modernc.org/sqlite"
)

// maxGapRows keeps the page small. The count of each gap is always exact.
const maxGapRows = 500

const (
	maxLossyArtists = 25
	maxRecent       = 10
)

var lossless = map[string]bool{
	"FLAC": true, "ALAC": true, "WAV": true, "WAVE": true, "AIFF": true,
	"APE": true, "WAVPACK": true, "MONKEY'S AUDIO": true,
}

// Class tells how far an album is on the way to an all-lossless library.
const (
	ClassLossless = "lossless" // FLAC, ALAC and other lossless formats
	ClassLossy    = "lossy"
	ClassMixed    = "mixed" // lossless and lossy tracks in one album
)

type Totals struct {
	Tracks       int     `json:"tracks"`
	Albums       int     `json:"albums"`
	Singletons   int     `json:"singletons"`
	Artists      int     `json:"artists"`
	Hours        float64 `json:"hours"`
	Bytes        int64   `json:"bytes"` // size on disk
	MissingFiles int     `json:"missingFiles"`
	HiRes        int     `json:"hiRes"` // lossless tracks above 16 bit or 48 kHz
}

// Progress counts albums and tracks per class.
type Progress struct {
	Albums map[string]int `json:"albums"`
	Tracks map[string]int `json:"tracks"`
}

// Group is a row of a chart that is split by class. Lossy includes mixed albums.
type Group struct {
	Name     string `json:"name"`
	Lossless int    `json:"lossless"`
	Lossy    int    `json:"lossy"`
}

type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Format struct {
	Name     string `json:"name"`
	Count    int    `json:"count"`
	Lossless bool   `json:"lossless"`
	Bytes    int64  `json:"bytes"`
}

// Growth is the estimated library size at the end of a month, by import date.
type Growth struct {
	Month    string `json:"month"`
	Lossless int64  `json:"lossless"`
	Lossy    int64  `json:"lossy"`
}

type Series struct {
	Name   string `json:"name"`
	Values []int  `json:"values"`
}

type Buckets struct {
	Labels []string `json:"labels"`
	Series []Series `json:"series"`
}

// Check is one metadata field: how many of the total miss it.
type Check struct {
	Label   string `json:"label"`
	Missing int    `json:"missing"`
	Total   int    `json:"total"`
}

// ArtistList is an artist with the albums that still need a lossless copy.
type ArtistList struct {
	Artist string   `json:"artist"`
	Total  int      `json:"total"` // all albums of the artist
	Albums []GapRow `json:"albums"`
}

// Recent is an album imported lately.
type Recent struct {
	ID     int    `json:"id"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
	Year   int    `json:"year"`
	Format string `json:"format"`
	Class  string `json:"class"`
	Added  string `json:"added"`
	Cover  string `json:"cover"` // file URL of the full size image, for a page opened from the same machine
	Thumb  string `json:"thumb"` // embedded copy for when the file URL does not load, empty when there is no readable cover
	URL    string `json:"url"`
}

type GapRow struct {
	ID     int    `json:"id"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
	Year   int    `json:"year"`
	Format string `json:"format"`
	Detail string `json:"detail"`
	URL    string `json:"url,omitempty"` // MusicBrainz or Discogs release
}

type Gap struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Hint  string   `json:"hint"`
	Count int      `json:"count"`
	Rows  []GapRow `json:"rows"`
}

// Data is everything the report page shows.
type Data struct {
	Generated    string           `json:"generated"`
	Totals       Totals           `json:"totals"`
	Progress     Progress         `json:"progress"`
	Formats      []Format         `json:"formats"` // tracks per format
	LossyBitrate Buckets          `json:"lossyBitrate"`
	Quality      []Count          `json:"quality"`      // lossless tracks per bit depth and sample rate
	SizesScanned string           `json:"sizesScanned"` // when the file sizes were last read from disk
	Recent       []Recent         `json:"recent"`
	AddedDays    []Count          `json:"addedDays"`
	AlbumTypes   []Group          `json:"albumTypes"`
	Labels       []Group          `json:"labels"`
	LossyArtists []ArtistList     `json:"lossyArtists"`
	Growth       []Growth         `json:"growth"`
	AlbumSizes   []Count          `json:"albumSizes"` // albums by track count
	Internal     Internal         `json:"internal"`
	Years        []Group          `json:"years"`
	Added        []Count          `json:"added"` // albums added per month
	Artists      []Group          `json:"artists"`
	Genres       []Group          `json:"genres"`
	Checks       []Check          `json:"checks"`
	Gaps         []Gap            `json:"gaps"`
	History      []store.Snapshot `json:"history"`
}

type album struct {
	id, year, tracks, singletonTracks int
	artist, name, genre               string
	hasArt, hasSource, folderArt      bool
	artpath, coverPath                string
	mbid, albumType, label            string
	discogs, missing                  int
	dirs                              []string
	added                             float64
	formats                           map[string]int
	bitrateSum                        int
	bytes                             int64
	noTitle, noTrack                  int
}

// Collect reads the beets database and computes all statistics. History is
// left empty. ignored reports album names that are left out everywhere.
func Collect(dbPath string, ignored func(string) bool, now time.Time, disk Disk) (Data, error) {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return Data{}, fmt.Errorf("open beets db: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`
SELECT
    i.album_id,
    CAST(i.path AS TEXT)         AS path,
    COALESCE(i.title, '')        AS title,
    COALESCE(i.artist, '')       AS artist,
    COALESCE(i.track, 0)         AS track,
    UPPER(COALESCE(i.format, '')) AS format,
    COALESCE(i.bitrate, 0)       AS bitrate,
    COALESCE(i.bitdepth, 0)      AS bitdepth,
    COALESCE(i.samplerate, 0)    AS samplerate,
    COALESCE(i.length, 0)        AS length,
    COALESCE(a.albumartist, '')  AS albumartist,
    COALESCE(a.album, '')        AS album,
    COALESCE(a.year, 0)          AS year,
    COALESCE(CAST(a.artpath AS TEXT), '') AS artpath,
    COALESCE(a.genre, '')        AS genre,
    COALESCE(a.mb_albumid, '')   AS mb_albumid,
    COALESCE(a.discogs_albumid, 0) AS discogs_albumid,
    COALESCE(a.albumtype, '')    AS albumtype,
    COALESCE(a.label, '')        AS label,
    COALESCE(a.added, 0)         AS added
FROM items i
LEFT JOIN albums a ON (a.id = i.album_id)
`)
	if err != nil {
		return Data{}, fmt.Errorf("query items: %w", err)
	}
	defer rows.Close()

	d := Data{Generated: now.Format("2006-01-02 15:04")}
	d.Progress = Progress{Albums: map[string]int{}, Tracks: map[string]int{}}
	albums := map[int]*album{}
	formats := map[string]int{}
	quality := map[string]int{}
	var singletons []GapRow
	var lossyBitrates []lossyTrack
	var seconds float64
	var totalBytes int64
	formatBytes := map[string]int64{}
	var tracks, noTitle, noTrack int

	for rows.Next() {
		var albumID sql.NullInt64
		var path, title, artist, format, albumArtist, albumName, artpath, genre, mbid, albumType, label string
		var track, bitrate, bitdepth, samplerate, year, discogs int
		var length, added float64
		if err = rows.Scan(&albumID, &path, &title, &artist, &track, &format, &bitrate, &bitdepth, &samplerate,
			&length, &albumArtist, &albumName, &year, &artpath, &genre, &mbid, &discogs, &albumType, &label, &added); err != nil {
			return Data{}, fmt.Errorf("scan item: %w", err)
		}
		if albumID.Valid && ignored != nil && ignored(albumName) {
			continue
		}
		if format == "" {
			format = "UNKNOWN"
		}
		tracks++
		seconds += length
		formats[format]++
		var size int64
		if size, err = disk.Size(path); err != nil {
			return Data{}, err
		}
		if size == 0 {
			d.Totals.MissingFiles++
		}
		totalBytes += size
		formatBytes[format] += size
		if title == "" {
			noTitle++
		}
		if track == 0 {
			noTrack++
		}
		if lossless[format] && (bitdepth > 16 || samplerate > 48000) {
			d.Totals.HiRes++
		}
		if lossless[format] {
			quality[qualityLabel(bitdepth, samplerate)]++
		} else {
			lossyBitrates = append(lossyBitrates, lossyTrack{format, bitrate})
		}

		if !albumID.Valid {
			d.Totals.Singletons++
			singletons = append(singletons, GapRow{Artist: artist, Album: title, Format: format})
			continue
		}
		a := albums[int(albumID.Int64)]
		if a == nil {
			// Beets can keep a full date like 20161101 in the year column, see LoadAlbums.
			if year > 9999 {
				year /= 10000
			}
			// Years before 1900 are tagging mistakes, count them as missing.
			if year < 1900 {
				year = 0
			}
			a = &album{
				id: int(albumID.Int64), year: year, artist: albumArtist, name: albumName, genre: genre,
				hasArt: artpath != "", artpath: artpath, hasSource: mbid != "" || discogs != 0, mbid: mbid, discogs: discogs,
				albumType: albumType, label: label, added: added, formats: map[string]int{},
			}
			albums[a.id] = a
		}
		a.tracks++
		if size == 0 {
			a.missing++
		}
		if dir := filepath.Dir(path); !contains(a.dirs, dir) {
			a.dirs = append(a.dirs, dir)
		}
		a.formats[format]++
		a.bitrateSum += bitrate
		a.bytes += size
		if title == "" {
			a.noTitle++
		}
		if track == 0 {
			a.noTrack++
		}
	}
	if err = rows.Err(); err != nil {
		return Data{}, err
	}

	d.Totals.Tracks = tracks
	d.Totals.Albums = len(albums)
	d.Totals.Hours = seconds / 3600
	d.Totals.Bytes = totalBytes
	for _, c := range sortedCounts(formats, 0) {
		d.Formats = append(d.Formats, Format{c.Name, c.Count, lossless[c.Name], formatBytes[c.Name]})
	}
	d.Quality = sortedCounts(quality, 8)
	d.LossyBitrate = bitrateBuckets(lossyBitrates)
	if d.Internal, err = collectInternal(db, dbPath); err != nil {
		return Data{}, err
	}
	for _, a := range albums {
		if a.hasArt {
			continue
		}
		for _, dir := range a.dirs {
			var name string
			if name, err = disk.CoverFile(dir); err != nil {
				return Data{}, err
			}
			if name != "" {
				a.folderArt, a.coverPath = true, filepath.Join(dir, name)
				break
			}
		}
	}
	if err = d.fillAlbums(albums, singletons, noTitle, noTrack); err != nil {
		return Data{}, err
	}
	return d, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// url links to the release page. The beets id field can also hold a Discogs or Bandcamp id.
func (a *album) url() string {
	if uuidPattern.MatchString(a.mbid) {
		return "https://musicbrainz.org/release/" + a.mbid
	}
	if a.discogs > 0 {
		return "https://www.discogs.com/release/" + strconv.Itoa(a.discogs)
	}
	return ""
}

type lossyTrack struct {
	format  string
	bitrate int
}

func (d *Data) fillAlbums(albums map[int]*album, singletons []GapRow, noTitle, noTrack int) error {
	list := make([]*album, 0, len(albums))
	for _, a := range albums {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].artist != list[j].artist {
			return strings.ToLower(list[i].artist) < strings.ToLower(list[j].artist)
		}
		if !strings.EqualFold(list[i].name, list[j].name) {
			return strings.ToLower(list[i].name) < strings.ToLower(list[j].name)
		}
		return list[i].id < list[j].id
	})

	years := map[int]*Group{}
	artists := map[string]*Group{}
	genres := map[string]*Group{}
	added := map[string]int{}
	monthBytes := map[string]*Growth{}
	sizeBuckets := make([]int, len(albumSizeEdges))
	groups := map[string][]*album{}
	rowOf := map[int]GapRow{}
	types := map[string]*Group{}
	labels := map[string]*Group{}
	days := map[string]int{}
	artistTotal := map[string]int{}
	lossyBy := map[string]*ArtistList{}
	artistNames := map[string]bool{}
	gaps := map[string]*Gap{}
	gapOrder := []struct{ key, label, hint string }{
		{"lossy", "Lossy albums", "MP3, AAC and other lossy files. Needs a lossless source."},
		{"mixed", "Mixed albums", "Lossless and lossy tracks in one album."},
		{"duplicates", "Duplicate albums", "Albums that share artist and name with another album."},
		{"missing", "Files missing on disk", "Beets lists the file, but it is not in the music folder."},
		{"noname", "No album name", "Album name is empty."},
		{"noyear", "No year", "Release year is missing."},
		{"noart", "No cover art", "No art file set in beets and no cover image in the album folder. Art embedded in the files is not detected."},
		{"folderart", "Cover in folder only", "A cover image is in the album folder, but beets has no art path for it. Run beet fetchart."},
		{"nogenre", "No genre", "Genre is empty."},
		{"nosource", "No MusicBrainz or Discogs ID", "Album was never matched to an online database."},
		{"notitle", "Tracks without title", "At least one track has no title."},
		{"notrack", "Tracks without track number", "At least one track has no track number."},
		{"singleton", "Singletons", "Tracks that belong to no album."},
	}
	for _, g := range gapOrder {
		gaps[g.key] = &Gap{Key: g.key, Label: g.label, Hint: g.hint}
	}
	addGap := func(key string, r GapRow) {
		g := gaps[key]
		g.Count++
		if len(g.Rows) < maxGapRows {
			g.Rows = append(g.Rows, r)
		}
	}

	var noName, noYear, noArt, noBeetsArt, noGenre, noSource int
	for _, a := range list {
		class, format := a.class()
		d.Progress.Albums[class]++
		d.Progress.Tracks[class] += a.tracks
		row := GapRow{ID: a.id, Artist: a.artist, Album: a.name, Year: a.year, Format: format, URL: a.url()}
		rowOf[a.id] = row

		bucket := func(m map[string]*Group, name string) {
			g := m[name]
			if g == nil {
				g = &Group{Name: name}
				m[name] = g
			}
			g.add(class)
		}
		if a.year > 0 {
			g := years[a.year]
			if g == nil {
				g = &Group{Name: strconv.Itoa(a.year)}
				years[a.year] = g
			}
			g.add(class)
		}
		if a.artist != "" {
			artistNames[a.artist] = true
			if !strings.EqualFold(a.artist, "Various Artists") {
				bucket(artists, a.artist)
			}
		}
		if a.genre != "" {
			bucket(genres, a.genre)
		}
		typeName := strings.ToLower(strings.TrimSpace(a.albumType))
		if typeName == "" {
			typeName = "(unset)"
		}
		bucket(types, typeName)
		if a.label != "" {
			bucket(labels, a.label)
		}
		if a.added > 0 {
			day := time.Unix(int64(a.added), 0).Format("2006-01-02")
			days[day]++
			month := day[:7]
			added[month]++
			g := monthBytes[month]
			if g == nil {
				g = &Growth{Month: month}
				monthBytes[month] = g
			}
			if class == ClassLossless {
				g.Lossless += a.bytes
			} else {
				g.Lossy += a.bytes
			}
		}
		i := 0
		for a.tracks > albumSizeEdges[i].max {
			i++
		}
		sizeBuckets[i]++
		if a.name != "" {
			key := strings.ToLower(a.artist) + "\x00" + strings.ToLower(a.name)
			groups[key] = append(groups[key], a)
		}
		if a.artist != "" {
			artistTotal[a.artist]++
		}

		switch class {
		case ClassLossy, ClassMixed:
			r := row
			r.Detail = fmt.Sprintf("%d kbps avg", a.bitrateSum/a.tracks/1000)
			addGap(class, r)
			if a.artist != "" && !strings.EqualFold(a.artist, "Various Artists") {
				l := lossyBy[a.artist]
				if l == nil {
					l = &ArtistList{Artist: a.artist}
					lossyBy[a.artist] = l
				}
				l.Albums = append(l.Albums, r)
			}
		}
		if a.name == "" {
			noName++
			addGap("noname", row)
		}
		if a.year == 0 {
			noYear++
			addGap("noyear", row)
		}
		if !a.hasArt {
			noBeetsArt++
		}
		if !a.hasArt && !a.folderArt {
			noArt++
			addGap("noart", row)
		}
		if a.folderArt {
			addGap("folderart", row)
		}
		if a.missing > 0 {
			r := row
			r.Detail = fmt.Sprintf("%d of %d files", a.missing, a.tracks)
			addGap("missing", r)
		}
		if a.genre == "" {
			noGenre++
			addGap("nogenre", row)
		}
		if !a.hasSource {
			noSource++
			addGap("nosource", row)
		}
		if a.noTitle > 0 {
			r := row
			r.Detail = fmt.Sprintf("%d of %d tracks", a.noTitle, a.tracks)
			addGap("notitle", r)
		}
		if a.noTrack > 0 {
			r := row
			r.Detail = fmt.Sprintf("%d of %d tracks", a.noTrack, a.tracks)
			addGap("notrack", r)
		}
	}
	for _, r := range singletons {
		addGap("singleton", r)
	}

	d.Totals.Artists = len(artistNames)
	n := d.Totals.Albums
	d.Checks = []Check{
		{"Album name", noName, n},
		{"Release year", noYear, n},
		{"Cover art in beets", noBeetsArt, n},
		{"Cover art (beets or folder)", noArt, n},
		{"Genre", noGenre, n},
		{"MusicBrainz / Discogs ID", noSource, n},
		{"Track title", noTitle, d.Totals.Tracks},
		{"Track number", noTrack, d.Totals.Tracks},
	}

	yearKeys := make([]int, 0, len(years))
	for y := range years {
		yearKeys = append(yearKeys, y)
	}
	sort.Ints(yearKeys)
	for _, y := range yearKeys {
		d.Years = append(d.Years, *years[y])
	}
	d.Artists = topGroups(artists, 15)
	d.Genres = topGroups(genres, 30)
	months := make([]string, 0, len(added))
	for m := range added {
		months = append(months, m)
	}
	sort.Strings(months)
	var run Growth
	for _, m := range months {
		d.Added = append(d.Added, Count{m, added[m]})
		run.Month = m
		run.Lossless += monthBytes[m].Lossless
		run.Lossy += monthBytes[m].Lossy
		d.Growth = append(d.Growth, run)
	}
	for i, e := range albumSizeEdges {
		d.AlbumSizes = append(d.AlbumSizes, Count{e.label, sizeBuckets[i]})
	}
	for _, g := range groups {
		if len(g) > 1 {
			d.Internal.DuplicateAlbums++
		}
	}
	for _, a := range list {
		if g := groups[strings.ToLower(a.artist)+"\x00"+strings.ToLower(a.name)]; a.name != "" && len(g) > 1 {
			r := rowOf[a.id]
			r.Detail = fmt.Sprintf("%d copies", len(g))
			addGap("duplicates", r)
		}
	}
	for _, g := range gapOrder {
		d.Gaps = append(d.Gaps, *gaps[g.key])
	}

	d.AlbumTypes = topGroups(types, 12)
	d.Labels = topGroups(labels, 15)
	dayKeys := make([]string, 0, len(days))
	for k := range days {
		dayKeys = append(dayKeys, k)
	}
	sort.Strings(dayKeys)
	for _, k := range dayKeys {
		d.AddedDays = append(d.AddedDays, Count{k, days[k]})
	}
	for _, l := range lossyBy {
		l.Total = artistTotal[l.Artist]
		d.LossyArtists = append(d.LossyArtists, *l)
	}
	sort.Slice(d.LossyArtists, func(i, j int) bool {
		a, b := d.LossyArtists[i], d.LossyArtists[j]
		if len(a.Albums) != len(b.Albums) {
			return len(a.Albums) > len(b.Albums)
		}
		return a.Artist < b.Artist
	})
	if len(d.LossyArtists) > maxLossyArtists {
		d.LossyArtists = d.LossyArtists[:maxLossyArtists]
	}

	recent := make([]*album, 0, len(list))
	for _, a := range list {
		if a.added > 0 {
			recent = append(recent, a)
		}
	}
	sort.Slice(recent, func(i, j int) bool {
		if recent[i].added != recent[j].added {
			return recent[i].added > recent[j].added
		}
		return recent[i].id > recent[j].id
	})
	if len(recent) > maxRecent {
		recent = recent[:maxRecent]
	}
	for _, a := range recent {
		cover := a.artpath
		if cover == "" {
			cover = a.coverPath
		}
		thumb, coverURL := "", ""
		if cover != "" {
			var err error
			if thumb, err = thumbnail(cover); err != nil {
				return err
			}
			if thumb != "" {
				coverURL = (&url.URL{Scheme: "file", Path: cover}).String()
			}
		}
		class, format := a.class()
		d.Recent = append(d.Recent, Recent{
			ID: a.id, Artist: a.artist, Album: a.name, Year: a.year, Format: format, Class: class,
			Added: time.Unix(int64(a.added), 0).Format("2006-01-02"), Cover: coverURL, Thumb: thumb, URL: a.url(),
		})
	}
	return nil
}

func (g *Group) add(class string) {
	switch class {
	case ClassLossless:
		g.Lossless++
	default:
		g.Lossy++
	}
}

// class returns the album class and its dominant format.
func (a *album) class() (class, format string) {
	var ll, lossy, best int
	for f, n := range a.formats {
		if lossless[f] {
			ll += n
		} else {
			lossy += n
		}
		if n > best || (n == best && f < format) {
			best, format = n, f
		}
	}
	switch {
	case lossy > 0 && ll > 0:
		return ClassMixed, format
	case lossy > 0:
		return ClassLossy, format
	}
	return ClassLossless, format
}

func topGroups(m map[string]*Group, n int) []Group {
	out := make([]Group, 0, len(m))
	for _, g := range m {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := out[i].total(), out[j].total()
		if ti != tj {
			return ti > tj
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func (g Group) total() int { return g.Lossless + g.Lossy }

// sortedCounts sorts by count. With max > 0 the rest is folded into "Other".
func sortedCounts(m map[string]int, max int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Name < out[j].Name
	})
	if max > 0 && len(out) > max {
		other := 0
		for _, c := range out[max:] {
			other += c.Count
		}
		out = append(out[:max], Count{"Other", other})
	}
	return out
}

func qualityLabel(bitdepth, samplerate int) string {
	if bitdepth == 0 || samplerate == 0 {
		return "Unknown"
	}
	return fmt.Sprintf("%d bit / %s kHz", bitdepth, strconv.FormatFloat(float64(samplerate)/1000, 'f', -1, 64))
}

var bitrateEdges = []struct {
	label string
	below int // kbps
}{
	{"<128", 128}, {"128-159", 160}, {"160-191", 192}, {"192-223", 224},
	{"224-255", 256}, {"256-309", 310}, {"310+", 1 << 30},
}

func bitrateBuckets(tracks []lossyTrack) Buckets {
	var b Buckets
	for _, e := range bitrateEdges {
		b.Labels = append(b.Labels, e.label)
	}
	byFormat := map[string][]int{}
	total := map[string]int{}
	for _, t := range tracks {
		kbps := t.bitrate / 1000
		i := 0
		for kbps >= bitrateEdges[i].below {
			i++
		}
		if byFormat[t.format] == nil {
			byFormat[t.format] = make([]int, len(bitrateEdges))
		}
		byFormat[t.format][i]++
		total[t.format]++
	}
	for _, c := range sortedCounts(total, 0) {
		b.Series = append(b.Series, Series{Name: c.Name, Values: byFormat[c.Name]})
	}
	return b
}

// Snapshot summarizes the data for the history in the store.
func (d Data) Snapshot() store.Snapshot {
	a := d.Progress.Albums
	return store.Snapshot{
		Day:            d.Generated[:10],
		Tracks:         d.Totals.Tracks,
		Albums:         d.Totals.Albums,
		LosslessAlbums: a[ClassLossless],
		LosslessTracks: d.Progress.Tracks[ClassLossless],
		Bytes:          d.Totals.Bytes,
		LossyAlbums:    a[ClassLossy] + a[ClassMixed],
	}
}

var albumSizeEdges = []struct {
	label string
	max   int // tracks
}{
	{"1", 1}, {"2-4", 4}, {"5-8", 8}, {"9-12", 12}, {"13-16", 16}, {"17-24", 24}, {"25+", 1 << 30},
}
