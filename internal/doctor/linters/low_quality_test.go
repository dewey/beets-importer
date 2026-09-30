package linters_test

import (
	"context"
	"strings"
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

const lowQualityThreshold = 128_000

func TestLowQualityFlagsBelowThreshold(t *testing.T) {
	items := []beets.Item{
		{Path: "/music/low.mp3", Format: "MP3", Bitrate: 96_000},
		{Path: "/music/ok.mp3", Format: "MP3", Bitrate: 320_000},
	}
	issues := runLowQuality(t, items)
	assertIssues(t, issues, []string{"/music/low.mp3"})
	if issues[0].Severity != doctor.SeverityWarning {
		t.Errorf("severity: got %q, want %q", issues[0].Severity, doctor.SeverityWarning)
	}
	if !strings.Contains(issues[0].Description, "below") {
		t.Errorf("description %q should mention the threshold", issues[0].Description)
	}
}

func TestLowQualityFlagsOldAAC(t *testing.T) {
	// 192 kbps AAC is above the threshold but below the 256 kbps AAC cutoff.
	items := []beets.Item{{Path: "/music/old.m4a", Format: "AAC", Bitrate: 192_000}}
	issues := runLowQuality(t, items)
	assertIssues(t, issues, []string{"/music/old.m4a"})
	if !strings.Contains(issues[0].Description, "old AAC") {
		t.Errorf("description %q should flag old AAC", issues[0].Description)
	}
}

func TestLowQualityIgnoresHighQuality(t *testing.T) {
	items := []beets.Item{
		{Path: "/music/a.mp3", Format: "MP3", Bitrate: 320_000},
		{Path: "/music/b.m4a", Format: "AAC", Bitrate: 256_000}, // exactly the AAC cutoff
		{Path: "/music/c.flac", Format: "FLAC", Bitrate: 1_000_000},
	}
	assertIssues(t, runLowQuality(t, items), nil)
}

func TestLowQualitySkipsUnknownBitrate(t *testing.T) {
	items := []beets.Item{
		{Path: "/music/a.flac", Format: "FLAC", Bitrate: 0},
		{Path: "/music/b.mp3", Format: "MP3", Bitrate: -1},
	}
	assertIssues(t, runLowQuality(t, items), nil)
}

func TestLowQualityContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	l := linters.NewLowQuality([]beets.Item{{Path: "/music/low.mp3", Format: "MP3", Bitrate: 96_000}}, lowQualityThreshold)
	issues, err := l.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected no issues after cancellation, got %d", len(issues))
	}
}

func runLowQuality(t *testing.T, items []beets.Item) []doctor.Issue {
	t.Helper()
	l := linters.NewLowQuality(items, lowQualityThreshold)
	issues, err := l.Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	return issues
}
