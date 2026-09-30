package linters_test

import (
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

func TestProtectedAudio(t *testing.T) {
	items := []beets.Item{
		{Path: "/music/Belaire/01-03 Back Into The Wall.m4p", Format: "AAC"},
		{Path: "/music/Non-Album/single.M4P", Format: "AAC"},
		{Path: "/music/Burial/01 Archangel.m4a", Format: "AAC"},
		{Path: "/music/Kode9/01 Nothing.flac", Format: "FLAC"},
	}
	issues, err := linters.NewProtectedAudio(items).Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	assertIssues(t, issues, []string{"/music/Belaire/01-03 Back Into The Wall.m4p", "/music/Non-Album/single.M4P"})
}
