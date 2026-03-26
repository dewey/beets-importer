package cmd

import "testing"

// ── formatAllowed ─────────────────────────────────────────────────────────────

func TestFormatAllowed_emptyAllowlistPassesEverything(t *testing.T) {
	for _, fmt := range []string{"MP3", "FLAC", "AAC", ""} {
		if !formatAllowed(fmt, nil) {
			t.Errorf("formatAllowed(%q, nil) = false, want true (no filter)", fmt)
		}
		if !formatAllowed(fmt, []string{}) {
			t.Errorf("formatAllowed(%q, []) = false, want true (no filter)", fmt)
		}
	}
}

func TestFormatAllowed_exactMatch(t *testing.T) {
	if !formatAllowed("MP3", []string{"MP3"}) {
		t.Error("MP3 should match allowlist [MP3]")
	}
}

func TestFormatAllowed_caseInsensitive(t *testing.T) {
	if !formatAllowed("mp3", []string{"MP3"}) {
		t.Error("lowercase mp3 should match uppercase MP3 in allowlist")
	}
	if !formatAllowed("FLAC", []string{"flac"}) {
		t.Error("uppercase FLAC should match lowercase flac in allowlist")
	}
}

func TestFormatAllowed_noMatch(t *testing.T) {
	if formatAllowed("OGG", []string{"MP3", "AAC"}) {
		t.Error("OGG should not match allowlist [MP3, AAC]")
	}
}

func TestFormatAllowed_multipleFormats(t *testing.T) {
	allowlist := []string{"MP3", "AAC", "FLAC"}
	for _, f := range allowlist {
		if !formatAllowed(f, allowlist) {
			t.Errorf("formatAllowed(%q, allowlist) = false, want true", f)
		}
	}
}
