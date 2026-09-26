package beets

import "testing"

func TestProcessedPaths(t *testing.T) {
	paths, err := ProcessedPaths("testdata/state.pickle")
	if err != nil {
		t.Fatal(err)
	}
	if !paths["/music/Ferris MC - 2001 - fertich!"] {
		t.Error("missing ASCII path")
	}
	// Stored as NFD (o + combining diaeresis), looked up as NFC.
	if !paths["/music/Björk - Post"] {
		t.Error("NFD path not normalized to NFC")
	}
	if !paths["/music/Nas - Illmatic"] {
		t.Error("parent of multi-disc folders not marked")
	}
	if paths["/music/Burial - Untrue"] {
		t.Error("unrelated folder marked")
	}
}

func TestProcessedPathsMissingFile(t *testing.T) {
	if _, err := ProcessedPaths("testdata/nonexistent.pickle"); err == nil {
		t.Error("expected error for missing state file")
	}
}
