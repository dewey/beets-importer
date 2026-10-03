package cmd

import (
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/compare"
	"github.com/dewey/beets-importer/internal/ignore"
	"github.com/dewey/beets-importer/internal/picker"
	"github.com/dewey/beets-importer/internal/source"
)

// ── formatAllowed ─────────────────────────────────────────────────────────────

func TestFormatAllowed_emptyAllowlistPassesEverything(t *testing.T) {
	for _, fmt := range []string{"MP3", "FLAC", "AAC", ""} {
		if !formatAllowed(fmt, nil) {
			t.Errorf("formatAllowed(%q, nil) = false, want true (no filter)", fmt)
		}
		if !formatAllowed(fmt, []string{}) {
			t.Errorf("formatAllowed(%q, []) = false, want true (no filter)", fmt)
		}
	}
}

func TestFormatAllowed_exactMatch(t *testing.T) {
	if !formatAllowed("MP3", []string{"MP3"}) {
		t.Error("MP3 should match allowlist [MP3]")
	}
}

func TestFormatAllowed_caseInsensitive(t *testing.T) {
	if !formatAllowed("mp3", []string{"MP3"}) {
		t.Error("lowercase mp3 should match uppercase MP3 in allowlist")
	}
	if !formatAllowed("FLAC", []string{"flac"}) {
		t.Error("uppercase FLAC should match lowercase flac in allowlist")
	}
}

func TestFormatAllowed_noMatch(t *testing.T) {
	if formatAllowed("OGG", []string{"MP3", "AAC"}) {
		t.Error("OGG should not match allowlist [MP3, AAC]")
	}
}

func TestFormatAllowed_multipleFormats(t *testing.T) {
	allowlist := []string{"MP3", "AAC", "FLAC"}
	for _, f := range allowlist {
		if !formatAllowed(f, allowlist) {
			t.Errorf("formatAllowed(%q, allowlist) = false, want true", f)
		}
	}
}

// ── ignores ───────────────────────────────────────────────────────────────────

func ignoreTestModel(t *testing.T, store *ignore.Store) upgradesModel {
	t.Helper()
	return upgradesModel{
		libraryAlbums: []beets.Album{
			{ID: 1, AlbumArtist: "BLACKPINK", Album: "Square Two", Format: "MP3", Path: "/lib/Square Two"},
			{ID: 2, AlbumArtist: "Keane", Album: "Hopes and Fears", Format: "MP3", Path: "/lib/Hopes"},
		},
		threshold: 0.7,
		scanCh:    make(chan tea.Msg, 1),
		ignores:   store,
	}
}

func scanMsg(dir, artist, album string) albumScannedMsg {
	return albumScannedMsg{album: source.Album{DirName: dir, Path: "/src/" + dir, Artist: artist, Album: album, Format: "FLAC"}}
}

func TestUpgradesModelSplitsIgnored(t *testing.T) {
	store, err := ignore.Load(filepath.Join(t.TempDir(), "ignore.json"))
	if err != nil {
		t.Fatal(err)
	}
	store.Set(ignore.Upgrades, ignore.Entry{Source: "SQUARE ONE", Library: "/lib/Square Two"}, true)

	m := ignoreTestModel(t, store)
	for _, msg := range []albumScannedMsg{
		scanMsg("SQUARE ONE", "BLACKPINK", "Square Two"),
		scanMsg("Hopes and Fears", "Keane", "Hopes and Fears"),
	} {
		next, _ := m.Update(msg)
		m = next.(upgradesModel)
	}

	if len(m.candidates) != 1 || m.candidates[0].Source.DirName != "Hopes and Fears" {
		t.Errorf("candidates = %+v, want only Hopes and Fears", m.candidates)
	}
	if len(m.ignored) != 1 || m.ignored[0].Source.DirName != "SQUARE ONE" {
		t.Errorf("ignored = %+v, want only SQUARE ONE", m.ignored)
	}
}

func TestUpgradesModelLimitIgnoresIgnoredCandidates(t *testing.T) {
	store, err := ignore.Load(filepath.Join(t.TempDir(), "ignore.json"))
	if err != nil {
		t.Fatal(err)
	}
	store.Set(ignore.Upgrades, ignore.Entry{Source: "SQUARE ONE", Library: "/lib/Square Two"}, true)

	m := ignoreTestModel(t, store)
	m.limit = 1
	next, _ := m.Update(scanMsg("SQUARE ONE", "BLACKPINK", "Square Two"))
	m = next.(upgradesModel)
	if m.phase == phaseDone {
		t.Error("an ignored candidate must not count toward the limit")
	}
}

func TestBuildPickerItemsGreysIgnoredLast(t *testing.T) {
	cand := func(dir string) compare.Candidate {
		return compare.Candidate{
			Source:  source.Album{DirName: dir, Path: "/src/" + dir},
			Library: beets.Album{Path: "/lib/" + dir},
		}
	}
	items := buildPickerItems([]compare.Candidate{cand("A")}, []compare.Candidate{cand("B")})
	if len(items) != 2 || items[0].Ignored || !items[1].Ignored || items[1].Name != "B" {
		t.Errorf("items = %+v, want A active then B ignored", items)
	}
}

func TestSaveIgnoresRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ignore.json")
	store, err := ignore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	store.Set(ignore.Upgrades, ignore.Entry{Source: "B", Library: "/lib/B"}, true)

	items := []picker.Item{
		{Name: "A", LibraryPath: "/lib/A", Ignored: true},
		{Name: "B", LibraryPath: "/lib/B", Ignored: false},
	}
	if err = saveIgnores(store, items); err != nil {
		t.Fatal(err)
	}

	store, err = ignore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !store.Has(ignore.Upgrades, ignore.Entry{Source: "A", Library: "/lib/A"}) {
		t.Error("A must be ignored after save")
	}
	if store.Has(ignore.Upgrades, ignore.Entry{Source: "B", Library: "/lib/B"}) {
		t.Error("B must be unignored after save")
	}
}
