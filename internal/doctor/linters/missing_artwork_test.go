package linters_test

import (
	"context"
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

func TestMissingArtworkFlagsAlbumsWithoutArt(t *testing.T) {
	albums := []beets.Album{
		{Path: "/music/a", HasArtwork: false},
		{Path: "/music/b", HasArtwork: true},
		{Path: "/music/c", HasArtwork: false},
	}
	issues := runMissingArtwork(t, albums)
	assertIssues(t, issues, []string{"/music/a", "/music/c"})
	for _, iss := range issues {
		if iss.Severity != doctor.SeverityWarning {
			t.Errorf("severity for %s: got %q, want %q", iss.Path, iss.Severity, doctor.SeverityWarning)
		}
	}
}

func TestMissingArtworkAllPresent(t *testing.T) {
	albums := []beets.Album{
		{Path: "/music/a", HasArtwork: true},
		{Path: "/music/b", HasArtwork: true},
	}
	assertIssues(t, runMissingArtwork(t, albums), nil)
}

func TestMissingArtworkContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	l := linters.NewMissingArtwork([]beets.Album{{Path: "/music/a"}})
	issues, err := l.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues after cancellation, got %d", len(issues))
	}
}

func runMissingArtwork(t *testing.T, albums []beets.Album) []doctor.Issue {
	t.Helper()
	l := linters.NewMissingArtwork(albums)
	issues, err := l.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	return issues
}
