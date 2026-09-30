package linters_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

func TestUntrackedDirsFlagsUntracked(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	writeFile(t, filepath.Join(album, "track.mp3"))

	issues := runUntracked(t, root, nil)
	assertIssues(t, issues, []string{album})
	if issues[0].Severity != doctor.SeverityError {
		t.Errorf("severity: got %q, want %q", issues[0].Severity, doctor.SeverityError)
	}
}

func TestUntrackedDirsSkipsTracked(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	writeFile(t, filepath.Join(album, "track.flac"))

	issues := runUntracked(t, root, []beets.Item{{Path: filepath.Join(album, "track.flac")}})
	assertIssues(t, issues, nil)
}

func TestUntrackedDirsDescendsThroughNonAudioParent(t *testing.T) {
	root := t.TempDir()
	// artist/ holds only a subdirectory; the audio lives one level deeper.
	nested := filepath.Join(root, "artist", "album")
	writeFile(t, filepath.Join(nested, "track.flac"))

	issues := runUntracked(t, root, nil)
	assertIssues(t, issues, []string{nested})
}

func TestUntrackedDirsIgnoresNonAudio(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "artwork", "cover.jpg"))
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	issues := runUntracked(t, root, nil)
	assertIssues(t, issues, nil)
}

func TestUntrackedDirsMatchesExtensionCaseInsensitively(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	writeFile(t, filepath.Join(album, "Track.MP3"))

	issues := runUntracked(t, root, nil)
	assertIssues(t, issues, []string{album})
}

func TestUntrackedDirsNormalisesKnownPaths(t *testing.T) {
	root := t.TempDir()
	album := filepath.Join(root, "album")
	writeFile(t, filepath.Join(album, "track.mp3"))

	unclean := root + "/./album/track.mp3"
	issues := runUntracked(t, root, []beets.Item{{Path: unclean}})
	assertIssues(t, issues, nil)
}

func TestUntrackedDirsSkipsMultiDisc(t *testing.T) {
	root := t.TempDir()
	cd1 := filepath.Join(root, "album", "CD1")
	cd2 := filepath.Join(root, "album", "CD2")
	writeFile(t, filepath.Join(cd1, "01.flac"))
	writeFile(t, filepath.Join(cd2, "01.flac"))

	issues := runUntracked(t, root, []beets.Item{
		{Path: filepath.Join(cd1, "01.flac")},
		{Path: filepath.Join(cd2, "01.flac")},
	})
	assertIssues(t, issues, nil)
}

func TestUntrackedDirsContextCancelled(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "album", "track.mp3"))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	l := linters.NewUntrackedDirs(root, nil)
	_, err := l.Run(ctx)
	if err == nil {
		t.Error("expected an error on cancelled context, got nil")
	}
}

// runUntracked runs the linter and fails the test on an unexpected error.
func runUntracked(t *testing.T, root string, items []beets.Item) []doctor.Issue {
	t.Helper()
	l := linters.NewUntrackedDirs(root, items)
	issues, err := l.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	return issues
}

// writeFile creates path and any missing parent directories.
func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
}
