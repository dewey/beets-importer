package source

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ── LoadCache ─────────────────────────────────────────────────────────────────

func TestLoadCache_nonexistentPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "cache.json")
	c, err := LoadCache(path)
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil cache")
	}
}

func TestLoadCache_emptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	os.WriteFile(path, []byte("{}"), 0o644)
	c, err := LoadCache(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil cache")
	}
}

func TestLoadCache_corruptJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	os.WriteFile(path, []byte("not json {{{{"), 0o644)
	c, err := LoadCache(path)
	// Corrupt data is silently discarded; no error returned.
	if err != nil {
		t.Fatalf("expected nil error for corrupt cache, got: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil (empty) cache after corrupt discard")
	}
}

// ── store / lookup ────────────────────────────────────────────────────────────

func TestCache_storeAndLookup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	c, _ := LoadCache(path)

	dir := "/some/dir"
	mtime := time.Unix(1000, 0)
	album := Album{DirName: "Test Album", Artist: "Artist", Format: "FLAC"}

	c.store(dir, mtime, false, album)
	got, ok := c.lookup(dir, mtime, false)
	if !ok {
		t.Fatal("expected cache hit after store")
	}
	if got.DirName != album.DirName || got.Artist != album.Artist {
		t.Errorf("lookup returned %+v, want %+v", got, album)
	}
}

func TestCache_missOnMtimeChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	c, _ := LoadCache(path)

	dir := "/some/dir"
	mtime1 := time.Unix(1000, 0)
	mtime2 := time.Unix(2000, 0)
	c.store(dir, mtime1, false, Album{DirName: "Old"})

	_, ok := c.lookup(dir, mtime2, false)
	if ok {
		t.Error("expected cache miss when mtime changes")
	}
}

func TestCache_missOnBitrateFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	c, _ := LoadCache(path)

	dir := "/some/dir"
	mtime := time.Unix(1000, 0)
	c.store(dir, mtime, false, Album{DirName: "NoBitrate"})

	// Stored without bitrate, look up with bitrate — should miss.
	_, ok := c.lookup(dir, mtime, true)
	if ok {
		t.Error("expected cache miss when withBitrate flag differs")
	}
}

func TestCache_separateKeysForBitrateFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	c, _ := LoadCache(path)

	dir := "/some/dir"
	mtime := time.Unix(1000, 0)
	albumNoBitrate := Album{DirName: "NoBitrate", Format: "FLAC"}
	albumWithBitrate := Album{DirName: "WithBitrate", Format: "FLAC", AvgBitrate: 900000}

	c.store(dir, mtime, false, albumNoBitrate)
	c.store(dir, mtime, true, albumWithBitrate)

	got, ok := c.lookup(dir, mtime, false)
	if !ok || got.DirName != "NoBitrate" {
		t.Errorf("no-bitrate lookup = %+v ok=%v, want NoBitrate", got, ok)
	}
	got, ok = c.lookup(dir, mtime, true)
	if !ok || got.DirName != "WithBitrate" {
		t.Errorf("with-bitrate lookup = %+v ok=%v, want WithBitrate", got, ok)
	}
}

// ── dirty / Save ──────────────────────────────────────────────────────────────

func TestCache_saveNoopWhenClean(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	os.WriteFile(path, []byte(`{}`), 0o644)

	info1, _ := os.Stat(path)
	c, _ := LoadCache(path)

	// No stores → dirty is false → Save is a no-op.
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info2, _ := os.Stat(path)
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Error("Save should be a no-op when cache is not dirty")
	}
}

func TestCache_saveWritesWhenDirty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	c, _ := LoadCache(path)

	c.store("/dir", time.Unix(1, 0), false, Album{DirName: "X"})
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("cache file not created: %v", err)
	}
}

func TestCache_saveCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "deep", "cache.json")
	c, _ := LoadCache(path)
	c.store("/d", time.Unix(1, 0), false, Album{DirName: "Y"})
	if err := c.Save(); err != nil {
		t.Fatalf("Save with nested dirs: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
}

// ── round-trip ────────────────────────────────────────────────────────────────

func TestCache_roundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	mtime := time.Unix(42, 0)
	want := Album{DirName: "Roundtrip", Artist: "A", Album: "B", Year: 2020, Format: "MP3"}

	// Write
	c1, _ := LoadCache(path)
	c1.store("/dir", mtime, false, want)
	if err := c1.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and verify
	c2, err := LoadCache(path)
	if err != nil {
		t.Fatalf("LoadCache after save: %v", err)
	}
	got, ok := c2.lookup("/dir", mtime, false)
	if !ok {
		t.Fatal("expected cache hit after reload")
	}
	if got.DirName != want.DirName || got.Artist != want.Artist || got.Year != want.Year {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestCache_saveNotDirtyAfterSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.json")
	c, _ := LoadCache(path)
	c.store("/d", time.Unix(1, 0), false, Album{DirName: "Z"})
	c.Save() //nolint:errcheck

	// Second Save should be a no-op (dirty cleared by first Save).
	info1, _ := os.Stat(path)
	c.Save() //nolint:errcheck
	info2, _ := os.Stat(path)
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Error("second Save should be a no-op after a successful Save")
	}
}
