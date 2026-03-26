package source

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/agnivade/levenshtein"
)

// fixture represents one real entry from the source directory together with
// the ground truth read from the embedded file tags via ffprobe.
// first_audio is recorded for documentation only; no files are read in these tests.
type fixture struct {
	dir        string
	firstAudio string // for reference only
	wantArtist string // from tags (album_artist preferred over artist)
	wantAlbum  string // from tags
	wantYear   int    // from tags
}

// Ground truth: 50 real entries from /Volumes/Archive/qbittorrent/data/torrents/music
// wantArtist/wantAlbum/wantYear are the values ffprobe returned from embedded tags.
// Empty wantArtist/wantAlbum means tags had no data for that field (untagged rip).
var fixtures = []fixture{
	// Tags had full data
	{"[1974] Crosswinds (2001 Atlantic 8122-73528-2)", "Billy Cobham - 01 - Spanish Moss.flac", "Billy Cobham", "Crosswinds", 1974},
	{"Celph Titled & Buckwild - 2010 - Nineteen Ninety Now [FLAC]", "01 - The Deal Maker.flac", "Celph Titled & Buckwild", "Nineteen Ninety Now", 2010},
	{"A$AP Rocky - At.Long.Last.A$AP (2015) - CD FLAC", "01 Holy Ghost.flac", "A$AP Rocky", "At.Long.Last.A$AP", 0},
	{"Hilltop Hoods - Drinking From The Sun (Limited Edtition)2012[FLAC]", "01 - The Thirst Pt. 1 (Interlude).flac", "Hilltop Hoods", "Drinking From The Sun (Limited Edtition)", 2012},
	{"Glenn Gould   Herbert Von Karajan - 2008] The Legendary Berlin Concert", "01. Ludwig Van Beethoven....flac", "Glenn Gould & Herbert Von Karajan", "The Legendary Berlin Concert", 2008},
	{"Burial - Truant Rough Sleeper FLAC", "01 Truant.flac", "Burial", "Truant / Rough Sleeper", 2012},
	{"Kid Cudi - Speedin' Bullet 2 Heaven (2015) [WEB-flac]", "Disc 1/01 - Edge Of The Earth.flac", "Kid Cudi", "Speedin' Bullet 2 Heaven", 2015},
	{"The Rapture - Pieces Of The People We Love", "01 - Don Gon Do It.flac", "The Rapture", "Pieces Of The People We Love", 2006},
	{"VA-Cut_Killer_Keep_It_Real-Remastered-CD-FLAC-2015-Mrflac", "01-cut_killer-intro.flac", "Cut Killer", "Cut Killer Keep It Real", 2015},
	{"After the Playboy Mansion [MP3-V2]", "Disc 1/01 Lil Louis.mp3", "Dimitri from Paris", "After the Playboy Mansion", 2002},
	{"Ghostface Killah - Supreme Clientele [2000-FLAC-Lossless-Log-Cue]", "Supreme Clientele - 01 - Intro.flac", "Ghostface Killah", "Supreme Clientele", 0},
	{"Guano_Apes_-_Offline-2014-MOD", "01_guano_apes_-_like_somebody.mp3", "Guano Apes", "Offline", 2014},
	{"Sunset bw Sequencer [MP3-V2]", "01 Track01.mp3", "Lifelike", "Sunset bw Sequencer", 2009},
	{"Various Artists - 2004 - The Very Best Of Cole Porter [FLAC]", "01 - Too Darn Hot.flac", "Various Artists", "The Very Best Of Cole Porter", 2004},
	{"Aaron Dilloway - Modern Jester CD (2012) [FLAC]", "01 Tremors.flac", "Aaron Dilloway", "Modern Jester", 2012},
	{"Fierce - So Long FLAC", "01 Fierce - So Long (Radio Edit).flac", "Fierce", "So Long (CD Single)", 1999},
	{"Myth Syzer - Porcelain (2015) [WEB-FLAC]", "01. Big Blue.flac", "Myth Syzer", "Porcelain", 2015},
	{"barry guy london jazz composers' orchestra - 1972 - ode", "LJCO - Ode - Disc One - 01 - Part I.flac", "LJCO", "Ode", 1972},
	{"Anita O'Day - Anita (1956) [FLAC]", "01 - You're The Top.flac", "Anita O'Day", "Anita", 1956},
	{"The Raconteurs - Broken Boy Soldiers[FLAC]", "01 - Steady As She Goes.flac", "The Raconteurs", "Broken Boy Soliders", 2006},
	{"Plaid-Parts_In_The_Post_(Plaid_Remixes)-2CD-FLAC-2003", "101-bjork-all_is_full_of_love_(plaid_remix).flac", "Björk", "Parts In The Post (Plaid Remixes)", 2003},
	{"Rose Elinor Dougall - 2010 - Without Why", "01 - Start,Stop,Synchro.flac", "Rose Elinor Dougall", "Without Why", 2010},
	{"VA - Vienna Scientists II (FLAC)", "01 Freedom Satellite - Easy 99.flac", "Various Artists", "Vienna Scientists II", 1999},
	{"Johnny Booth - The Sagua EP (2009) [320]", "01 Brainwash.mp3", "Johnny Booth", "The Sagua EP", 2009},
	{"17 Hippies - (2009) - El Dorado", "17 Hippies - 01 - UZ.flac", "17 Hippies", "El Dorado", 2009},
	{"Die_Nerven-Fun-(TCM032)-DE-CD-FLAC-2014-k4", "01-die_nerven-albtraum.flac", "Die Nerven", "Fun", 2014},
	{"Underworld - Strawberry Hotel (2024) [FLAC CD] [UWR00098]", "01 - Underworld - Black Poppies.flac", "Underworld", "Strawberry Hotel", 2024},
	{"Pretty Lights - I Know the Truth [Single] (2011) [FLAC]", "01 I Know the Truth.flac", "Pretty Lights", "I Know the Truth [Single]", 2011},
	{"Marcus Mumford & T Bone Burnett - Inside Llewyn Davis  (2013) [FLAC]", "01 Hang Me, Oh Hang Me.flac", "Marcus Mumford & T Bone Burnett", "Inside Llewyn Davis", 2013},
	{"John Wayne - John Wayne's West In Music & Poster Art (2009) [FLAC]", "Disc 1/01 - The Alamo - Overture.flac", "John Wayne", "John Wayne's West In Music & Poster Art", 2009},
	{"Gentle Giant (1975) Free Hand {what.cd, Rock, FLAC}", "01 - Just The Same.flac", "Gentle Giant", "Free Hand", 1975},
	{"Magnetic Man - Magnetic Man [Deluxe Edition] (2010)", "CD1/01 - Flying Into Tokyo.flac", "Magnetic Man", "Magnetic Man (Deluxe Edition)", 2010},
	{"Burial - Rival Dealer [2013] [HDB080] [WEB] [FLAC]", "01. Burial - Rival Dealer.flac", "Burial", "Rival Dealer", 2013},
	{"Booba - Ouest Side (2006) [FLAC]", "01 - Mauvais Garçon.flac", "Booba", "Ouest Side", 2006},
	{"Brasstronaut - Mount Chimaera (2010) [FLAC]", "01 - Brasstronaut - Slow Knots.flac", "Brasstronaut", "Mount Chimaera", 2010},
	{"Mint Julep - Save Your Season (2011) [FLAC]", "01 - Chasing the Wind.flac", "Mint Julep", "Save Your Season", 2011},
	{"Mary Anne Hobbs (2009, FLAC-XLD-CUE) - Wild Angels", "01 - Mark Pritchard - _.flac", "Mary Anne Hobbs", "Wild Angels", 2009},
	{"Various Artists - 21st Century Ska 2001 FLAC", "01 Freaky Beanz - Pearly King Skank.flac", "Various Artists", "21st Century Ska 2001", 2001},
	{"Jacques Greene - Ready EP (2012) [FLAC]", "1 Ready.flac", "Jacques Greene", "Ready EP", 2012},
	{"Andrew Douglas Rothbard - 2006 - Abandoned Meander", "01 - A Beginning.flac", "Andrew Douglas Rothbard", "Abandoned Meander", 2006},
	{"Saltillo - 2006 - Ganglion", "01 - A Necessary End.flac", "Saltillo", "Ganglion", 2006},
	{"The Heights Band", "We Are The Heights Band/01 - My Baby.flac", "The Heights Band", "We Are The Heights Band", 2009},
	{"Coldplay-Parachutes-2000", "01-Coldplay-Don't_Panic.mp3", "Coldplay", "Parachutes", 2000},
	// Tags were empty (untagged / transcoded rips) — dir parsing is the only option
	{"Johannes Enders - Dome [Q8]", "01.The Essence Of A Day Part 1.ogg", "", "", 0},
	{"DM-FTHTF", "01 - Fragile Tension (Radio Mix).flac", "", "", 0},
	{"Blue October UK - Walk Amongst The Living [OGG]", "01 - The Miracle's Gone.ogg", "", "", 0},
	{"Tim McGraw - 2001 - Set This Circus Down [Q8]", "01 - The Cowboy In Me.ogg", "", "", 0},
	{"Gilles Peterson & Rainer Truby (1997) Talkin' Jazz Volume III [Q8]", "01 - Jazz Meets India , Yaad.ogg", "", "", 0},
	{"Depeche Mode - Tel Aviv 10.05.2009", "Disc 1/01 - In Chains.flac", "", "", 0},
	{"Neo - 2003 - Kontroll (A Filmzene) [AAC]", "01 - Metro.m4a", "", "", 0},
}

// --- helpers (mirrors of the real similarity logic) ---

var testPunctRe = regexp.MustCompile(`[^\w\s]`)

func normStr(s string) string {
	s = strings.ToLower(s)
	s = testPunctRe.ReplaceAllString(s, " ")
	var b strings.Builder
	prev := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prev {
				b.WriteRune(' ')
			}
			prev = true
		} else {
			b.WriteRune(r)
			prev = false
		}
	}
	return strings.TrimSpace(b.String())
}

func sim(a, b string) float64 {
	a, b = normStr(a), normStr(b)
	if a == b {
		return 1
	}
	if a == "" || b == "" {
		return 0
	}
	dist := levenshtein.ComputeDistance(a, b)
	max := len([]rune(a))
	if lb := len([]rune(b)); lb > max {
		max = lb
	}
	return 1 - float64(dist)/float64(max)
}

// goodMatch returns true if both artist and album are "close enough" to ground truth.
// We use a lenient threshold (0.75) because minor tagging variants are expected.
const matchThreshold = 0.75

func artistMatch(got, want string) bool { return sim(got, want) >= matchThreshold }
func albumMatch(got, want string) bool  { return sim(got, want) >= matchThreshold }

// TestParseDirNameVsTags compares the dir-name strategy against the tag ground truth
// and reports per-case and aggregate results.
func TestParseDirNameVsTags(t *testing.T) {
	type result struct {
		dir         string
		wantArtist  string
		wantAlbum   string
		wantYear    int
		dirArtist   string
		dirAlbum    string
		dirYear     int
		artistOK    bool
		albumOK     bool
		yearOK      bool
		untagged    bool // wantArtist == "" means tags were empty; dir-only case
	}

	var results []result
	for _, f := range fixtures {
		da, dal, dy := parseDirName(f.dir)

		r := result{
			dir:        f.dir,
			wantArtist: f.wantArtist,
			wantAlbum:  f.wantAlbum,
			wantYear:   f.wantYear,
			dirArtist:  da,
			dirAlbum:   dal,
			dirYear:    dy,
			untagged:   f.wantArtist == "" && f.wantAlbum == "",
		}
		if !r.untagged {
			r.artistOK = artistMatch(da, f.wantArtist)
			r.albumOK = albumMatch(dal, f.wantAlbum)
			r.yearOK = f.wantYear == 0 || dy == f.wantYear
		}
		results = append(results, r)
	}

	// Count tagged cases only (those with ground truth from tags)
	tagged := 0
	artistHits, albumHits, yearHits := 0, 0, 0
	for _, r := range results {
		if r.untagged {
			continue
		}
		tagged++
		if r.artistOK {
			artistHits++
		}
		if r.albumOK {
			albumHits++
		}
		if r.yearOK {
			yearHits++
		}
	}

	// Print per-case breakdown
	t.Log("\n=== DIR-NAME PARSING vs TAG GROUND TRUTH ===")
	t.Logf("%-55s  %-22s  %-22s  A  B  Y", "Directory", "Dir Artist→Want", "Dir Album→Want")
	t.Log(strings.Repeat("-", 120))
	for _, r := range results {
		if r.untagged {
			t.Logf("%-55s  [untagged — dir-only fallback]", trunc(r.dir, 55))
			continue
		}
		aOK := "✓"
		if !r.artistOK {
			aOK = "✗"
		}
		bOK := "✓"
		if !r.albumOK {
			bOK = "✗"
		}
		yOK := "✓"
		if !r.yearOK {
			yOK = "✗"
		}
		t.Logf("%-55s  %-22s  %-22s  %s  %s  %s",
			trunc(r.dir, 55),
			trunc(r.dirArtist+"→"+r.wantArtist, 22),
			trunc(r.dirAlbum+"→"+r.wantAlbum, 22),
			aOK, bOK, yOK,
		)
	}
	t.Log(strings.Repeat("-", 120))
	t.Logf("Dir-name strategy on %d tagged cases: artist %d/%d (%.0f%%)  album %d/%d (%.0f%%)  year %d/%d (%.0f%%)",
		tagged,
		artistHits, tagged, 100*float64(artistHits)/float64(tagged),
		albumHits, tagged, 100*float64(albumHits)/float64(tagged),
		yearHits, tagged, 100*float64(yearHits)/float64(tagged),
	)
	t.Log("\nNote: tags strategy scores 100% on all tagged fields by definition.")
	t.Logf("      %d/%d cases were untagged — dir-name is the only fallback for those.",
		len(fixtures)-tagged, len(fixtures))
}

// ── scanDir ──────────────────────────────────────────────────────────────────

func TestScanDir_emptyDir(t *testing.T) {
	dir := t.TempDir()
	_, ok, err := scanDir(dir, "Empty Album", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false for directory with no audio files")
	}
}

func TestScanDir_formatDetection(t *testing.T) {
	// 3 MP3s + 1 FLAC → dominant format is MP3.
	dir := t.TempDir()
	for _, name := range []string{"01.mp3", "02.mp3", "03.mp3", "bonus.flac"} {
		mustTouch(t, filepath.Join(dir, name))
	}
	album, ok, err := scanDir(dir, "Artist - Album", false, nil)
	if err != nil || !ok {
		t.Fatalf("expected scan ok, got err=%v ok=%v", err, ok)
	}
	if album.Format != "MP3" {
		t.Errorf("format = %q, want MP3 (3 MP3s beat 1 FLAC)", album.Format)
	}
	if album.TrackCount != 4 {
		t.Errorf("track count = %d, want 4", album.TrackCount)
	}
}

func TestScanDir_singleFlac(t *testing.T) {
	dir := t.TempDir()
	mustTouch(t, filepath.Join(dir, "track.flac"))
	album, ok, err := scanDir(dir, "Artist - Album", false, nil)
	if err != nil || !ok {
		t.Fatalf("expected scan ok, got err=%v ok=%v", err, ok)
	}
	if album.Format != "FLAC" {
		t.Errorf("format = %q, want FLAC", album.Format)
	}
	if album.TrackCount != 1 {
		t.Errorf("track count = %d, want 1", album.TrackCount)
	}
}

func TestScanDir_artworkDetection(t *testing.T) {
	dir := t.TempDir()
	mustTouch(t, filepath.Join(dir, "01.mp3"))
	mustTouch(t, filepath.Join(dir, "cover.jpg"))
	album, ok, err := scanDir(dir, "Artist - Album", false, nil)
	if err != nil || !ok {
		t.Fatalf("expected scan ok, got err=%v ok=%v", err, ok)
	}
	if !album.HasArtwork {
		t.Error("HasArtwork should be true when cover.jpg is present")
	}
}

func TestScanDir_noArtwork(t *testing.T) {
	dir := t.TempDir()
	mustTouch(t, filepath.Join(dir, "01.mp3"))
	album, ok, err := scanDir(dir, "Artist - Album", false, nil)
	if err != nil || !ok {
		t.Fatalf("expected scan ok, got err=%v ok=%v", err, ok)
	}
	if album.HasArtwork {
		t.Error("HasArtwork should be false when no artwork file is present")
	}
}

func TestScanDir_multiDiscSubdir(t *testing.T) {
	// Audio in Disc 1/ and Disc 2/ subdirs should both be counted.
	dir := t.TempDir()
	for _, sub := range []string{"Disc 1", "Disc 2"} {
		subdir := filepath.Join(dir, sub)
		os.Mkdir(subdir, 0o755)
		mustTouch(t, filepath.Join(subdir, "01.flac"))
	}
	album, ok, err := scanDir(dir, "Multi Disc Album", false, nil)
	if err != nil || !ok {
		t.Fatalf("expected scan ok, got err=%v ok=%v", err, ok)
	}
	if album.TrackCount != 2 {
		t.Errorf("track count = %d, want 2 (one per disc)", album.TrackCount)
	}
	if album.Format != "FLAC" {
		t.Errorf("format = %q, want FLAC", album.Format)
	}
}

func TestScanDir_dirNameUsedForMetadata(t *testing.T) {
	// Empty audio files have no tags → dir name is the only metadata source.
	dir := t.TempDir()
	mustTouch(t, filepath.Join(dir, "01.flac"))
	album, ok, err := scanDir(dir, "Burial - Untrue (2007) [FLAC]", false, nil)
	if err != nil || !ok {
		t.Fatalf("expected scan ok, got err=%v ok=%v", err, ok)
	}
	if album.Artist != "Burial" {
		t.Errorf("artist = %q, want Burial", album.Artist)
	}
	if album.Album != "Untrue" {
		t.Errorf("album = %q, want Untrue", album.Album)
	}
	if album.Year != 2007 {
		t.Errorf("year = %d, want 2007", album.Year)
	}
}

// ── ScanEach ─────────────────────────────────────────────────────────────────

func TestScanEach_callsFnForAudioDirs(t *testing.T) {
	root := t.TempDir()
	// Two dirs with audio files, one without.
	for _, name := range []string{"Album A", "Album B"} {
		sub := filepath.Join(root, name)
		os.Mkdir(sub, 0o755)
		mustTouch(t, filepath.Join(sub, "01.flac"))
	}
	noAudio := filepath.Join(root, "No Audio")
	os.Mkdir(noAudio, 0o755)
	mustTouch(t, filepath.Join(noAudio, "readme.txt"))

	var found []string
	err := ScanEach(context.Background(), root, ScanOptions{}, func(a Album, done, total int) bool {
		found = append(found, a.DirName)
		return true
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(found) != 2 {
		t.Errorf("expected 2 albums, got %d: %v", len(found), found)
	}
}

func TestScanEach_stopEarlyWhenFnReturnsFalse(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"A", "B", "C"} {
		sub := filepath.Join(root, name)
		os.Mkdir(sub, 0o755)
		mustTouch(t, filepath.Join(sub, "01.flac"))
	}
	calls := 0
	ScanEach(context.Background(), root, ScanOptions{}, func(a Album, done, total int) bool {
		calls++
		return calls < 2 // stop after first album
	})
	if calls != 2 {
		// fn is called, returns false — ScanEach stops. The call that returned false is counted.
		t.Errorf("expected fn called 2 times (last returns false), got %d", calls)
	}
}

func TestScanEach_progressCounters(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"A", "B", "C"} {
		sub := filepath.Join(root, name)
		os.Mkdir(sub, 0o755)
		mustTouch(t, filepath.Join(sub, "01.flac"))
	}
	var dones, totals []int
	ScanEach(context.Background(), root, ScanOptions{}, func(a Album, done, total int) bool {
		dones = append(dones, done)
		totals = append(totals, total)
		return true
	})
	// total should be the same (3) for every call
	for i, tot := range totals {
		if tot != 3 {
			t.Errorf("call %d: total = %d, want 3", i, tot)
		}
	}
	// done should be monotonically increasing
	for i := 1; i < len(dones); i++ {
		if dones[i] <= dones[i-1] {
			t.Errorf("done not increasing: dones[%d]=%d <= dones[%d]=%d", i, dones[i], i-1, dones[i-1])
		}
	}
}

func TestScanEach_onWarnCalledWhenDirUnreadable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission checks")
	}
	root := t.TempDir()
	restricted := filepath.Join(root, "locked")
	os.Mkdir(restricted, 0o000)
	defer os.Chmod(restricted, 0o755) // restore for t.TempDir cleanup

	var warned []string
	ScanEach(context.Background(), root, ScanOptions{
		OnWarn: func(dirName string, err error) {
			warned = append(warned, dirName)
		},
	}, func(a Album, done, total int) bool { return true })

	if len(warned) != 1 || warned[0] != "locked" {
		t.Errorf("expected warning for 'locked', got %v", warned)
	}
}

// mustTouch creates an empty file, failing the test on error.
func mustTouch(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("mustTouch %s: %v", path, err)
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s + strings.Repeat(" ", n-len(r))
	}
	return string(r[:n-1]) + "…"
}
