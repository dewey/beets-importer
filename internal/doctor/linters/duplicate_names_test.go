package linters

import (
	"testing"
	"testing/fstest"

	"github.com/dewey/beets-importer/internal/doctor"
)

// Real NFC/NFD pairs cannot be created on APFS, so the walk runs on a MapFS.
func TestDuplicateNames(t *testing.T) {
	fsys := fstest.MapFS{
		"Fiva/02 - geh\u00f6rt.flac":          {},
		"Fiva/02 - geho\u0308rt.flac":         {},
		"Fiva/03 - Zeitloopen.flac":           {},
		"Ros\u00eda/01.flac":                  {},
		"Rosi\u0301a/01.flac":                 {},
		"Burial - Untrue/01 - Archangel.flac": {},
	}
	var issues []doctor.Issue
	if err := walkDuplicateNames(t.Context(), fsys, "/music", ".", &issues); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(issues))
	for _, iss := range issues {
		got[iss.Path] = true
	}
	if len(issues) != 2 || !got["/music/Fiva"] || !got["/music"] {
		t.Errorf("got %v, want issues for /music/Fiva and /music", issues)
	}
}
