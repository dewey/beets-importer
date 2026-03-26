package compare

import (
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/source"
)

func TestFormatRank(t *testing.T) {
	lossless := []string{"FLAC", "ALAC", "WAV", "AIFF", "APE"}
	lossy := []string{"MP3", "AAC", "OGG", "OPUS"}
	unknown := []string{"", "WMA", "xyz"}

	for _, f := range lossless {
		if formatRank(f) != 3 {
			t.Errorf("formatRank(%q) = %d, want 3", f, formatRank(f))
		}
	}
	for _, f := range lossy {
		if formatRank(f) != 2 {
			t.Errorf("formatRank(%q) = %d, want 2", f, formatRank(f))
		}
	}
	for _, f := range unknown {
		if formatRank(f) != 1 {
			t.Errorf("formatRank(%q) = %d, want 1", f, formatRank(f))
		}
	}
	// Case-insensitive
	if formatRank("flac") != 3 {
		t.Errorf("formatRank(flac) should be 3 (case-insensitive)")
	}
}

func TestEvaluate_formatUpgrade(t *testing.T) {
	src := source.Album{Format: "FLAC", AvgBitrate: 900_000}
	lib := beets.Album{Format: "MP3", AvgBitrate: 320_000}

	c, ok := Evaluate(src, lib, 0.9, 32, false)
	if !ok {
		t.Fatal("expected upgrade candidate, got none")
	}
	if len(c.Reasons) == 0 {
		t.Error("expected at least one reason")
	}
	if c.Score != 0.9 {
		t.Errorf("score = %.2f, want 0.9", c.Score)
	}
}

func TestEvaluate_bitrateUpgrade(t *testing.T) {
	src := source.Album{Format: "MP3", AvgBitrate: 320_000}
	lib := beets.Album{Format: "MP3", AvgBitrate: 192_000}

	c, ok := Evaluate(src, lib, 0.9, 32, false)
	if !ok {
		t.Fatal("expected upgrade candidate, got none")
	}
	if len(c.Reasons) == 0 {
		t.Error("expected at least one reason")
	}
}

func TestEvaluate_noUpgrade_lowerBitrate(t *testing.T) {
	src := source.Album{Format: "MP3", AvgBitrate: 128_000}
	lib := beets.Album{Format: "MP3", AvgBitrate: 320_000}

	_, ok := Evaluate(src, lib, 0.9, 32, false)
	if ok {
		t.Error("expected no upgrade when source bitrate is lower")
	}
}

func TestEvaluate_noUpgrade_sameFormat_belowDelta(t *testing.T) {
	// delta = 10kbps < minBitrateDelta 32
	src := source.Album{Format: "MP3", AvgBitrate: 202_000}
	lib := beets.Album{Format: "MP3", AvgBitrate: 192_000}

	_, ok := Evaluate(src, lib, 0.9, 32, false)
	if ok {
		t.Error("expected no upgrade when bitrate delta is below minimum")
	}
}

func TestEvaluate_noUpgrade_downgradedFormat(t *testing.T) {
	src := source.Album{Format: "MP3", AvgBitrate: 320_000}
	lib := beets.Album{Format: "FLAC", AvgBitrate: 900_000}

	_, ok := Evaluate(src, lib, 0.9, 32, false)
	if ok {
		t.Error("expected no upgrade when source format is lower quality")
	}
}

func TestEvaluate_missingBitrate_skipsComparison(t *testing.T) {
	// Same format tier but one has unknown bitrate — no upgrade
	src := source.Album{Format: "MP3", AvgBitrate: 0}
	lib := beets.Album{Format: "MP3", AvgBitrate: 320_000}

	_, ok := Evaluate(src, lib, 0.9, 32, false)
	if ok {
		t.Error("expected no upgrade when source bitrate is unknown")
	}
}

func TestEvaluate_yearMatches(t *testing.T) {
	src := source.Album{Format: "FLAC", Year: 2005}
	lib := beets.Album{Format: "MP3", Year: 2005}
	c, ok := Evaluate(src, lib, 0.9, 32, false)
	if !ok {
		t.Fatal("expected upgrade candidate")
	}
	if !c.YearMatches {
		t.Error("YearMatches should be true when both years are equal")
	}
}

func TestEvaluate_yearMismatch(t *testing.T) {
	src := source.Album{Format: "FLAC", Year: 1981}
	lib := beets.Album{Format: "MP3", Year: 1996}
	c, ok := Evaluate(src, lib, 0.9, 32, false)
	if !ok {
		t.Fatal("expected upgrade candidate")
	}
	if c.YearMatches {
		t.Error("YearMatches should be false when years differ")
	}
}

func TestEvaluate_yearUnknown(t *testing.T) {
	// Missing year on either side → YearMatches must be false, not assumed true.
	cases := []struct {
		srcYear, libYear int
	}{
		{0, 2005},
		{2005, 0},
		{0, 0},
	}
	for _, tc := range cases {
		src := source.Album{Format: "FLAC", Year: tc.srcYear}
		lib := beets.Album{Format: "MP3", Year: tc.libYear}
		c, ok := Evaluate(src, lib, 0.9, 32, false)
		if !ok {
			t.Fatalf("expected upgrade candidate for years (%d, %d)", tc.srcYear, tc.libYear)
		}
		if c.YearMatches {
			t.Errorf("YearMatches should be false when srcYear=%d libYear=%d", tc.srcYear, tc.libYear)
		}
	}
}

func TestEvaluate_requireYearMatch_rejectsOnMismatch(t *testing.T) {
	src := source.Album{Format: "FLAC", Year: 1981}
	lib := beets.Album{Format: "MP3", Year: 1996}
	_, ok := Evaluate(src, lib, 0.9, 32, true)
	if ok {
		t.Error("expected candidate to be rejected when years differ and requireYearMatch=true")
	}
}

func TestEvaluate_requireYearMatch_allowsOnMatch(t *testing.T) {
	src := source.Album{Format: "FLAC", Year: 2005}
	lib := beets.Album{Format: "MP3", Year: 2005}
	_, ok := Evaluate(src, lib, 0.9, 32, true)
	if !ok {
		t.Error("expected candidate to pass when years match and requireYearMatch=true")
	}
}

func TestEvaluate_requireYearMatch_allowsWhenYearMissing(t *testing.T) {
	// requireYearMatch only rejects when BOTH sides have a year — missing year is not a disqualifier.
	cases := []struct{ srcYear, libYear int }{
		{0, 1996},
		{1981, 0},
		{0, 0},
	}
	for _, tc := range cases {
		src := source.Album{Format: "FLAC", Year: tc.srcYear}
		lib := beets.Album{Format: "MP3", Year: tc.libYear}
		_, ok := Evaluate(src, lib, 0.9, 32, true)
		if !ok {
			t.Errorf("expected candidate to pass when srcYear=%d libYear=%d (one side unknown)", tc.srcYear, tc.libYear)
		}
	}
}

func TestEvaluate_reasonText(t *testing.T) {
	src := source.Album{Format: "FLAC"}
	lib := beets.Album{Format: "MP3"}

	c, ok := Evaluate(src, lib, 1.0, 32, false)
	if !ok {
		t.Fatal("expected upgrade")
	}
	reason := c.Reasons[0]
	if reason != "FLAC replaces MP3" {
		t.Errorf("unexpected reason: %q", reason)
	}
}

func TestEvaluate_bothBitratesZero_noUpgrade(t *testing.T) {
	// Same format tier, both bitrates unknown — no upgrade can be determined.
	src := source.Album{Format: "MP3", AvgBitrate: 0}
	lib := beets.Album{Format: "MP3", AvgBitrate: 0}
	_, ok := Evaluate(src, lib, 0.9, 32, false)
	if ok {
		t.Error("expected no upgrade when both bitrates are 0 (unknown)")
	}
}

func TestEvaluate_sourceFormatUnknown_noUpgrade(t *testing.T) {
	// Unknown format (rank 1) cannot upgrade a lossy format (rank 2).
	src := source.Album{Format: ""}
	lib := beets.Album{Format: "MP3"}
	_, ok := Evaluate(src, lib, 0.9, 32, false)
	if ok {
		t.Error("expected no upgrade when source format is unknown (rank 1) vs MP3 (rank 2)")
	}
}

func TestEvaluate_bitrateReasonText(t *testing.T) {
	src := source.Album{Format: "MP3", AvgBitrate: 320_000}
	lib := beets.Album{Format: "MP3", AvgBitrate: 192_000}
	c, ok := Evaluate(src, lib, 0.9, 32, false)
	if !ok {
		t.Fatal("expected upgrade")
	}
	reason := c.Reasons[0]
	if reason != "320kbps replaces 192kbps" {
		t.Errorf("unexpected reason: %q", reason)
	}
}

func TestEvaluate_scorePreservedInCandidate(t *testing.T) {
	src := source.Album{Format: "FLAC"}
	lib := beets.Album{Format: "MP3"}
	c, ok := Evaluate(src, lib, 0.77, 32, false)
	if !ok {
		t.Fatal("expected upgrade")
	}
	if c.Score != 0.77 {
		t.Errorf("score = %.3f, want 0.77", c.Score)
	}
}
