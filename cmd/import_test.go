package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// --- readImportLog ---

func TestReadImportLog_missingFile(t *testing.T) {
	lines := readImportLog("/nonexistent/path/to/log.txt")
	if lines != nil {
		t.Errorf("expected nil for missing file, got %v", lines)
	}
}

func TestReadImportLog_readsLines(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("/music/Artist - Album\n/music/Another - One\n")
	f.Close()

	lines := readImportLog(f.Name())
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "/music/Artist - Album" {
		t.Errorf("unexpected line 0: %q", lines[0])
	}
}

// --- inLog ---

func TestInLog_found(t *testing.T) {
	lines := []string{
		"/Volumes/Archive/music/Burial - Untrue",
		"/Volumes/Archive/music/Coldplay - Parachutes",
	}
	if !inLog("Burial - Untrue", lines) {
		t.Error("expected Burial - Untrue to be found in log")
	}
}

func TestInLog_notFound(t *testing.T) {
	lines := []string{"/Volumes/Archive/music/Coldplay - Parachutes"}
	if inLog("Burial - Untrue", lines) {
		t.Error("expected Burial - Untrue NOT to be found in log")
	}
}

func TestInLog_emptyLog(t *testing.T) {
	if inLog("anything", nil) {
		t.Error("expected false for empty log")
	}
}

func TestInLog_partialMatchDoesNotFire(t *testing.T) {
	// "Burial" should not match a dir named "Burial - Untrue" if lines only contain "Burial"
	// (needle is "/" + dirName, so partial prefix doesn't count)
	lines := []string{"/Volumes/Archive/music/Burial"}
	if inLog("Burial - Untrue", lines) {
		t.Error("partial dir name should not match")
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
