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
