package linters_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dewey/beets-importer/internal/doctor"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

func TestEmptyDirsEmpty(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "empty")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	issues := runEmptyDirs(t, root)
	assertIssues(t, issues, []string{dir})
}

func TestEmptyDirsWithFiles(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "has_file")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "track.mp3"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	issues := runEmptyDirs(t, root)
	assertIssues(t, issues, nil)
}

func TestEmptyDirsNestedEmpty(t *testing.T) {
	root := t.TempDir()
	// parent/empty_a and parent/empty_b are both empty; parent itself is not.
	emptyA := filepath.Join(root, "parent", "empty_a")
	emptyB := filepath.Join(root, "parent", "empty_b")
	if err := os.MkdirAll(emptyA, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(emptyB, 0o755); err != nil {
		t.Fatal(err)
	}

	issues := runEmptyDirs(t, root)
	// parent should NOT be reported; only the two leaf empty dirs should.
	assertIssues(t, issues, []string{emptyA, emptyB})
}

func TestEmptyDirsMixedSiblings(t *testing.T) {
	root := t.TempDir()

	emptyDir := filepath.Join(root, "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fullDir := filepath.Join(root, "full")
	if err := os.Mkdir(fullDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fullDir, "song.flac"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	issues := runEmptyDirs(t, root)
	assertIssues(t, issues, []string{emptyDir})
}

func TestEmptyDirsContextCancelled(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a", "b"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // cancel immediately

	l := linters.NewEmptyDirs(root)
	_, err := l.Run(ctx)
	if err == nil {
		t.Error("expected an error on cancelled context, got nil")
	}
}

// runEmptyDirs is a test helper that runs the linter and fails on error.
func runEmptyDirs(t *testing.T, root string) []doctor.Issue {
	t.Helper()
	l := linters.NewEmptyDirs(root)
	issues, err := l.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	return issues
}

// assertIssues checks that the reported paths exactly match want (order-independent).
func assertIssues(t *testing.T, issues []doctor.Issue, want []string) {
	t.Helper()
	got := make(map[string]bool, len(issues))
	for _, iss := range issues {
		got[iss.Path] = true
	}
	wantSet := make(map[string]bool, len(want))
	for _, p := range want {
		wantSet[p] = true
	}
	for p := range wantSet {
		if !got[p] {
			t.Errorf("expected issue for %s, but it was not reported", p)
		}
	}
	for p := range got {
		if !wantSet[p] {
			t.Errorf("unexpected issue for %s", p)
		}
	}
	if len(issues) != len(want) {
		t.Errorf("issue count: got %d, want %d", len(issues), len(want))
	}
}
