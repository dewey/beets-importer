package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSnapshotsReplaceSameDay(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "sub", "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	for _, sn := range []Snapshot{
		{Day: "2026-10-02", Albums: 10, LosslessAlbums: 4},
		{Day: "2026-10-01", Albums: 9, LosslessAlbums: 3},
		{Day: "2026-10-02", Albums: 11, LosslessAlbums: 5, LosslessTracks: 50, Bytes: 123},
	} {
		if err = s.SaveSnapshot(sn); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Snapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Day != "2026-10-01" || got[1].Albums != 11 || got[1].LosslessTracks != 50 || got[1].Bytes != 123 {
		t.Errorf("got %+v", got)
	}
}

func TestDiskFacts(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	_, at, err := s.DiskFacts()
	if err != nil || !at.IsZero() {
		t.Fatalf("empty store: %v %v", at, err)
	}

	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	if err = s.SaveDiskFacts(DiskFacts{Sizes: map[string]int64{"/a": 1, "/b": 2}, Covers: map[string]string{"/d": "cover.jpg"}}, true, now); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveDiskFacts(DiskFacts{Sizes: map[string]int64{"/c": 3}}, false, now.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	f, at, err := s.DiskFacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Sizes) != 3 || f.Covers["/d"] != "cover.jpg" || !at.Equal(now) {
		t.Errorf("after add: %+v %v", f, at)
	}

	if err = s.SaveDiskFacts(DiskFacts{Sizes: map[string]int64{"/z": 9}}, true, now.AddDate(0, 1, 0)); err != nil {
		t.Fatal(err)
	}
	f, _, err = s.DiskFacts()
	if err != nil || len(f.Sizes) != 1 || len(f.Covers) != 0 {
		t.Errorf("after replace: %+v %v", f, err)
	}
}
