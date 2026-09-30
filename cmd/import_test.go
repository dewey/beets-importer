package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dewey/beets-importer/internal/beets"
)

// --- readPathsFromFile ---

func TestReadPathsFromFile_plainText(t *testing.T) {
	content := "/music/Album A\n/music/Album B\n\n# comment\n/music/Album C\n"
	paths, err := readPathsFromFileNamed(strings.NewReader(content), "paths.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 {
		t.Fatalf("expected 3 paths, got %d: %v", len(paths), paths)
	}
	if paths[1] != "/music/Album B" {
		t.Errorf("unexpected path[1]: %q", paths[1])
	}
}

func TestReadPathsFromFile_csv(t *testing.T) {
	content := "source_path,source_dir,score,year_matches\n" +
		"/music/Album A,Album A,0.95,true\n" +
		"/music/Album B,Album B,0.88,false\n"
	paths, err := readPathsFromFileNamed(strings.NewReader(content), "candidates.csv")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d: %v", len(paths), paths)
	}
	if paths[0] != "/music/Album A" {
		t.Errorf("unexpected paths[0]: %q", paths[0])
	}
	if paths[1] != "/music/Album B" {
		t.Errorf("unexpected paths[1]: %q", paths[1])
	}
}

func TestReadPathsFromFile_csvCaseInsensitiveExt(t *testing.T) {
	content := "source_path,score\n/music/Album A,0.95\n"
	paths, err := readPathsFromFileNamed(strings.NewReader(content), "output.CSV")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 || paths[0] != "/music/Album A" {
		t.Errorf("unexpected paths: %v", paths)
	}
}

func TestReadPathsFromFile_csvNoSourcePathColumn(t *testing.T) {
	content := "wrong_col,score\n/music/Album A,0.95\n"
	_, err := readPathsFromFileNamed(strings.NewReader(content), "out.csv")
	if err == nil {
		t.Error("expected error when 'source_path' column not found in CSV")
	}
}

func TestReadPathsFromFile_txtFileWithCsvContent(t *testing.T) {
	// .txt extension → plain text mode even if content looks like CSV
	content := "source_path,score\n/music/Album A,0.95\n"
	paths, err := readPathsFromFileNamed(strings.NewReader(content), "out.txt")
	if err != nil {
		t.Fatal(err)
	}
	// Treated as raw paths, so both lines are returned as-is
	if len(paths) != 2 {
		t.Fatalf("expected 2 raw lines, got %d: %v", len(paths), paths)
	}
}

func TestReadPathsFromFile_empty(t *testing.T) {
	paths, err := readPathsFromFileNamed(strings.NewReader(""), "paths.txt")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 paths for empty input, got %d", len(paths))
	}
}

// --- listDirsByMtime ---

func TestListDirsByMtime_sortedNewestFirst(t *testing.T) {
	root := t.TempDir()

	// Create dirs with distinct mtimes
	names := []string{"alpha", "beta", "gamma"}
	for i, name := range names {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		mtime := time.Now().Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	dirs, err := listDirsByMtime(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 3 {
		t.Fatalf("expected 3 dirs, got %d", len(dirs))
	}
	// gamma was modified last (i=2), so it should be first
	if dirs[0].name != "gamma" {
		t.Errorf("expected gamma first, got %q", dirs[0].name)
	}
	if dirs[2].name != "alpha" {
		t.Errorf("expected alpha last, got %q", dirs[2].name)
	}
}

func TestListDirsByMtime_skipsFiles(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "a-dir"), 0o755)
	os.WriteFile(filepath.Join(root, "a-file.txt"), []byte("x"), 0o644)

	dirs, err := listDirsByMtime(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0].name != "a-dir" {
		t.Errorf("expected only a-dir, got %v", dirs)
	}
}

func TestListDirsByMtime_skipsIgnoredAndTorrents(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"ignored", "torrents", "real-album"} {
		os.Mkdir(filepath.Join(root, name), 0o755)
	}

	dirs, err := listDirsByMtime(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0].name != "real-album" {
		t.Errorf("expected only real-album, got %v", dirs)
	}
}

func TestListDirsByMtime_emptyDir(t *testing.T) {
	root := t.TempDir()
	dirs, err := listDirsByMtime(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 0 {
		t.Errorf("expected 0 dirs, got %d", len(dirs))
	}
}

func TestReadAlbumIDs(t *testing.T) {
	ids, err := readAlbumIDs(strings.NewReader("248\n\n# split albums\n 782 \n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != 248 || ids[1] != 782 {
		t.Errorf("ids = %v, want [248 782]", ids)
	}
}

func TestReadAlbumIDsRejectsPaths(t *testing.T) {
	if _, err := readAlbumIDs(strings.NewReader("248\n/music/Album A\n")); err == nil {
		t.Error("expected error for a path in an ID list")
	}
}

func TestSortOutBatch(t *testing.T) {
	log := "import started Sun Sep 27 18:00:00 2026\n" +
		"skip /music/Kode9/Nothing [2015]\n" +
		"asis /music/Burial/Untrue [2007]\n" +
		"skip /music/Nas/Illmatic [1994]/CD1; /music/Nas/Illmatic [1994]/CD2\n"
	skipped := skippedFolders(log)
	if len(skipped) != 3 || !skipped["/music/Nas/Illmatic [1994]/CD2"] {
		t.Fatalf("skipped = %v", skipped)
	}

	// 1 was applied and is gone. 2 and 4 were skipped, 4 is a multi-disc
	// album. 3 was never reached.
	left := []beets.LibraryAlbum{{ID: 2}, {ID: 3}, {ID: 4}}
	folders := map[int][]string{
		2: {"/music/Kode9/Nothing [2015]"},
		3: {"/music/Björk/Post [1995]"},
		4: {"/music/Nas/Illmatic [1994]/CD1", "/music/Nas/Illmatic [1994]/CD2"},
	}
	skipIDs, untouched := sortOutBatch(left, folders, skipped)
	if len(skipIDs) != 2 || skipIDs[0] != 2 || skipIDs[1] != 4 {
		t.Errorf("skipIDs = %v, want [2 4]", skipIDs)
	}
	if len(untouched) != 1 || untouched[0] != 3 {
		t.Errorf("untouched = %v, want [3]", untouched)
	}
	if q := idQuery([]int{2, 4}); q != "id::^(2|4)$" {
		t.Errorf("idQuery = %q", q)
	}
}
