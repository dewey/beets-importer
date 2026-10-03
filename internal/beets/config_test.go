package beets_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
)

func TestLibraryDir(t *testing.T) {
	beet := filepath.Join(t.TempDir(), "beet")
	script := "#!/bin/sh\nprintf 'library: /x/lib.db\\ndirectory: /music/data\\n'\n"
	if err := os.WriteFile(beet, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := beets.LibraryDir(beet)
	if err != nil {
		t.Fatalf("LibraryDir() error: %v", err)
	}
	if got != "/music/data" {
		t.Errorf("got %q, want /music/data", got)
	}
}

func TestLibraryDirMissing(t *testing.T) {
	beet := filepath.Join(t.TempDir(), "beet")
	if err := os.WriteFile(beet, []byte("#!/bin/sh\necho 'plugins: []'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := beets.LibraryDir(beet); err == nil {
		t.Error("expected error when directory is not set")
	}
}

func TestReadSettings(t *testing.T) {
	dir := t.TempDir()
	beet := filepath.Join(dir, "beet")
	script := "#!/bin/sh\nif [ \"$2\" = -p ]; then echo " + dir + "/config.yaml; else printf 'library: library.db\\nstatefile: /abs/state.pickle\\n'; fi\n"
	if err := os.WriteFile(beet, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := beets.ReadSettings(beet)
	if err != nil {
		t.Fatalf("ReadSettings() error: %v", err)
	}
	if got.Library != filepath.Join(dir, "library.db") || got.StateFile != "/abs/state.pickle" {
		t.Errorf("got %+v", got)
	}
}
