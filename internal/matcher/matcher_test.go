package matcher

import (
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/source"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Hello World", "hello world"},
		{"AC/DC", "ac dc"},
		{"  multiple   spaces  ", "multiple spaces"},
		{"", ""},
		{"Björk", "björk"},                          // ö is a Unicode letter, preserved
		{"It's a Test!", "it s a test"},
		{"אמא אני לא רוצה להגמל", "אמא אני לא רוצה להגמל"}, // Hebrew preserved
		{"張國榮 / 哥哥的前半生", "張國榮 哥哥的前半生"},              // Chinese preserved, slash stripped
	}
	for _, c := range cases {
		got := normalize(c.in)
		if got != c.want {
			t.Errorf("normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSimilarity(t *testing.T) {
	cases := []struct {
		a, b      string
		wantExact bool // true = expect 1.0
		wantZero  bool // true = expect 0.0
	}{
		{"Burial", "Burial", true, false},
		{"", "", false, true}, // both empty = no usable text, treat as no match
		{"Burial", "", false, true},
		{"", "Burial", false, true},
	}
	for _, c := range cases {
		got := similarity(c.a, c.b)
		if c.wantExact && got != 1.0 {
			t.Errorf("similarity(%q, %q) = %.3f, want 1.0", c.a, c.b, got)
		}
		if c.wantZero && got != 0.0 {
			t.Errorf("similarity(%q, %q) = %.3f, want 0.0", c.a, c.b, got)
		}
		if got < 0 || got > 1 {
			t.Errorf("similarity(%q, %q) = %.3f out of [0,1]", c.a, c.b, got)
		}
	}

	// Near-identical strings should score high
	if s := similarity("Burial", "Buriel"); s < 0.7 {
		t.Errorf("similarity(Burial, Buriel) = %.3f, want >= 0.7", s)
	}
	// Completely different strings should score low
	if s := similarity("AAAA", "ZZZZ"); s > 0.3 {
		t.Errorf("similarity(AAAA, ZZZZ) = %.3f, want <= 0.3", s)
	}
}

func TestFindMatches_exact(t *testing.T) {
	src := []source.Album{
		{Artist: "Burial", Album: "Untrue"},
	}
	lib := []beets.Album{
		{AlbumArtist: "Burial", Album: "Untrue"},
	}
	matches := FindMatches(src, lib, 0.85)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Source.Album != "Untrue" {
		t.Errorf("unexpected source album: %q", matches[0].Source.Album)
	}
	if matches[0].Score < 0.85 {
		t.Errorf("score %.3f below threshold", matches[0].Score)
	}
}

func TestFindMatches_noMatch(t *testing.T) {
	src := []source.Album{
		{Artist: "Burial", Album: "Untrue"},
	}
	lib := []beets.Album{
		{AlbumArtist: "Coldplay", Album: "Parachutes"},
	}
	matches := FindMatches(src, lib, 0.85)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}
}

func TestFindMatches_belowThreshold(t *testing.T) {
	src := []source.Album{
		{Artist: "Burial", Album: "Untrue"},
	}
	lib := []beets.Album{
		{AlbumArtist: "Burial", Album: "Untrue"},
	}
	// Threshold of 1.0 should only pass perfect normalized matches
	matches := FindMatches(src, lib, 1.0)
	// "burial untrue" normalized matches exactly, so this should still be 1
	if len(matches) != 1 {
		t.Errorf("expected 1 match at threshold 1.0, got %d", len(matches))
	}
}

func TestFindMatches_oneToOne(t *testing.T) {
	// Two source albums that both best-match the same library album —
	// only the higher-scoring one should be matched (greedy, no reuse).
	src := []source.Album{
		{Artist: "Burial", Album: "Untrue"},
		{Artist: "Burial", Album: "Untrue"},
	}
	lib := []beets.Album{
		{AlbumArtist: "Burial", Album: "Untrue"},
	}
	matches := FindMatches(src, lib, 0.85)
	if len(matches) != 1 {
		t.Errorf("expected 1 match (library album used only once), got %d", len(matches))
	}
}

// TestSimilarity_nonLatinScriptsMismatch is the regression test for the false-positive
// bug where non-Latin text (Hebrew, Chinese, etc.) was stripped to empty strings by the
// ASCII-only \w regex, causing similarity("","") == 1.0 for completely unrelated albums.
func TestSimilarity_nonLatinScriptsMismatch(t *testing.T) {
	// These two strings share no content; similarity should be very low.
	hebrew := "אמא אני לא רוצה להגמל"
	chinese := "哥哥的前半生"
	got := similarity(hebrew, chinese)
	if got > 0.3 {
		t.Errorf("similarity(Hebrew, Chinese) = %.3f, want <= 0.3 (was falsely 1.0 before the fix)", got)
	}
}

// TestFindMatches_nonLatinNoFalsePositive verifies that albums with entirely different
// non-Latin scripts are not matched even at a low threshold.
func TestFindMatches_nonLatinNoFalsePositive(t *testing.T) {
	src := []source.Album{
		{Artist: "", Album: "אמא אני לא רוצה להגמל"}, // Hebrew
	}
	lib := []beets.Album{
		{AlbumArtist: "張國榮", Album: "哥哥的前半生"}, // Chinese — completely different
	}
	matches := FindMatches(src, lib, 0.70)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches between Hebrew and Chinese album, got %d (score %.2f)",
			len(matches), matches[0].Score)
	}
}

func TestFindMatches_yearMismatchWithinThreeLowersScore(t *testing.T) {
	// Perfect name match but years differ by ≤3 — should still match but score < 1.0.
	src := []source.Album{{Artist: "Burial", Album: "Untrue", Year: 2007}}
	lib := []beets.Album{{AlbumArtist: "Burial", Album: "Untrue", Year: 2009}}
	matches := FindMatches(src, lib, 0.70)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match for 2-year gap, got %d", len(matches))
	}
	if matches[0].Score >= 1.0 {
		t.Errorf("score should be < 1.0 when years differ, got %.3f", matches[0].Score)
	}
	// With weights 0.35+0.55+0.0 the max for a year mismatch is 0.90.
	if matches[0].Score > 0.90+1e-9 {
		t.Errorf("score %.3f exceeds expected max of 0.90 for year mismatch", matches[0].Score)
	}
}

func TestFindMatches_yearGapOverThreeSkipsMatch(t *testing.T) {
	// Year gap > 3 — pairs should not be considered the same album.
	src := []source.Album{{Artist: "Burial", Album: "Untrue", Year: 2007}}
	lib := []beets.Album{{AlbumArtist: "Burial", Album: "Untrue", Year: 2022}}
	matches := FindMatches(src, lib, 0.70)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for 15-year gap, got %d (score %.3f)", len(matches), matches[0].Score)
	}
}

func TestFindMatches_yearMatchKeepsFullScore(t *testing.T) {
	// Perfect name match and matching year — score should be 1.0.
	src := []source.Album{{Artist: "Burial", Album: "Untrue", Year: 2007}}
	lib := []beets.Album{{AlbumArtist: "Burial", Album: "Untrue", Year: 2007}}
	matches := FindMatches(src, lib, 0.85)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Score != 1.0 {
		t.Errorf("score = %.3f, want 1.0 when names and year all match", matches[0].Score)
	}
}

func TestFindMatches_yearUnknownFallsBackToNameOnly(t *testing.T) {
	// No year on either side — score formula is identical to before (name-only weights).
	src := []source.Album{{Artist: "Burial", Album: "Untrue"}}
	lib := []beets.Album{{AlbumArtist: "Burial", Album: "Untrue"}}
	matches := FindMatches(src, lib, 0.85)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Score != 1.0 {
		t.Errorf("score = %.3f, want 1.0 when year is unknown on both sides", matches[0].Score)
	}
}

func TestFindMatches_emptyLibrary(t *testing.T) {
	src := []source.Album{{Artist: "Burial", Album: "Untrue"}}
	matches := FindMatches(src, nil, 0.85)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches with empty library, got %d", len(matches))
	}
}

func TestFindMatches_noArtist(t *testing.T) {
	// When source has no artist, score should be album-only
	src := []source.Album{
		{Artist: "", Album: "Untrue"},
	}
	lib := []beets.Album{
		{AlbumArtist: "Burial", Album: "Untrue"},
	}
	matches := FindMatches(src, lib, 0.85)
	if len(matches) != 1 {
		t.Errorf("expected 1 match, got %d", len(matches))
	}
}

func TestFindMatches_emptySource(t *testing.T) {
	lib := []beets.Album{{AlbumArtist: "Burial", Album: "Untrue"}}
	matches := FindMatches(nil, lib, 0.85)
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for empty source, got %d", len(matches))
	}
}

func TestFindMatches_thresholdZeroAlwaysMatches(t *testing.T) {
	// Threshold 0 means any score >= 0 is accepted — even completely different albums.
	src := []source.Album{{Artist: "Burial", Album: "Untrue"}}
	lib := []beets.Album{{AlbumArtist: "Coldplay", Album: "Parachutes"}}
	matches := FindMatches(src, lib, 0.0)
	if len(matches) != 1 {
		t.Errorf("expected 1 match at threshold 0.0, got %d", len(matches))
	}
}

func TestFindMatches_multipleSourceCompeteForOneLibraryAlbum(t *testing.T) {
	// Two different source albums both best-match the same library album.
	// Only the first one processed wins (greedy); the second gets no match.
	src := []source.Album{
		{Artist: "Burial", Album: "Untrue"},           // exact match → score 1.0
		{Artist: "Burial", Album: "Untrue (Deluxe)"}, // near match → score < 1.0
	}
	lib := []beets.Album{
		{AlbumArtist: "Burial", Album: "Untrue"},
	}
	matches := FindMatches(src, lib, 0.70)
	if len(matches) != 1 {
		t.Errorf("expected 1 match (library album used once), got %d", len(matches))
	}
	if matches[0].Source.Album != "Untrue" {
		t.Errorf("expected the exact match to win, got %q", matches[0].Source.Album)
	}
}
