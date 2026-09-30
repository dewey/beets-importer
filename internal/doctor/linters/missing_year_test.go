package linters_test

import (
	"context"
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

func TestMissingYearFlagsUnsetYear(t *testing.T) {
	albums := []beets.Album{
		{Path: "/music/a", Year: 0},
		{Path: "/music/b", Year: 1999},
		{Path: "/music/c", Year: 0},
	}
	issues := runMissingYear(t, albums)
	assertIssues(t, issues, []string{"/music/a", "/music/c"})
	for _, iss := range issues {
		if iss.Severity != doctor.SeverityWarning {
			t.Errorf("severity for %s: got %q, want %q", iss.Path, iss.Severity, doctor.SeverityWarning)
		}
	}
}

func TestMissingYearAllSet(t *testing.T) {
	albums := []beets.Album{
		{Path: "/music/a", Year: 2001},
		{Path: "/music/b", Year: 1984},
	}
	assertIssues(t, runMissingYear(t, albums), nil)
}

func TestMissingYearContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	l := linters.NewMissingYear([]beets.Album{{Path: "/music/a", Year: 0}})
	issues, err := l.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues after cancellation, got %d", len(issues))
	}
}

func runMissingYear(t *testing.T, albums []beets.Album) []doctor.Issue {
	t.Helper()
	l := linters.NewMissingYear(albums)
	issues, err := l.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	return issues
}
