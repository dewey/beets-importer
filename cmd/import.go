package cmd

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/picker"
	"github.com/dewey/beets-importer/internal/source"
	"github.com/spf13/cobra"
	"golang.org/x/text/unicode/norm"
)

var (
	flagImportFile     string
	flagImportLimit    int
	flagImportSince    string
	flagImportReimport bool
	flagImportLibrary  bool
)

// retagField is the flexible field set on every album retagged with
// --library. beets keeps it, so progress survives between runs.
const retagField = "retagged"

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Pick recently added unimported albums and import them, or import from a file of paths or album IDs",
	RunE:  runImport,
}

func init() {
	addDBFlag(importCmd)
	addSourceFlag(importCmd)
	addBeetFlag(importCmd)
	addScanFlags(importCmd)
	importCmd.Flags().StringVar(&flagStateFile, "state-file", "",
		"Path to the beets incremental state file (state.pickle); folders beets already processed are skipped")
	importCmd.Flags().StringVar(&flagImportFile, "from-file", "",
		"Import the album paths in this file instead of opening the picker")
	importCmd.Flags().IntVar(&flagImportLimit, "limit", 0,
		"Maximum number of albums to process (0 = no limit)")
	importCmd.Flags().StringVar(&flagImportSince, "since", "",
		"Only show albums added on or after this date (YYYY-MM-DD)")
	importCmd.Flags().BoolVar(&flagImportReimport, "reimport", false,
		"Also list folders beets already processed, and run beet with --noincremental so it imports them again")
	importCmd.Flags().BoolVar(&flagImportLibrary, "library", false,
		"Read beets album IDs from --from-file and retag them with 'beet import -L'")
	importCmd.MarkFlagsMutuallyExclusive("from-file", "since")
	importCmd.MarkFlagsMutuallyExclusive("library", "reimport")
}

func runImport(_ *cobra.Command, _ []string) error {
	if flagImportLibrary {
		return runRetagFromFile()
	}
	if flagImportFile != "" {
		return runImportFromFile()
	}
	return runImportLatest()
}

func runImportLatest() error {
	if err := requireFlag("source", flagSource); err != nil {
		return err
	}
	if err := requireFlag("beet", flagBeet); err != nil {
		return err
	}
	if err := requireFlag("state-file", flagStateFile); err != nil {
		return err
	}

	var since time.Time
	if flagImportSince != "" {
		t, err := time.Parse("2006-01-02", flagImportSince)
		if err != nil {
			return fmt.Errorf("--since: expected YYYY-MM-DD, got %q", flagImportSince)
		}
		since = t
	}

	processed, err := beets.ProcessedPaths(flagStateFile)
	if err != nil {
		return err
	}

	var spinMsg atomic.Value
	spinMsg.Store("Listing source directory…")
	stop := startSpinner(&spinMsg)
	dirs, err := listDirsByMtime(flagSource)
	stop()
	if err != nil {
		return err
	}

	spinMsg.Store("Filtering unimported albums…")
	stop = startSpinner(&spinMsg)

	var scanCache *source.ScanCache
	if !flagNoCache {
		if cachePath, err := source.DefaultCachePath(); err == nil {
			scanCache, _ = source.LoadCache(cachePath)
		}
	}

	limit := flagImportLimit
	if limit == 0 {
		limit = len(dirs)
	}

	var candidates []source.Album
	for _, d := range dirs {
		if len(candidates) >= limit {
			break
		}
		if !since.IsZero() && d.mtime.Before(since) {
			continue
		}
		dirPath := filepath.Join(flagSource, d.name)
		if !flagImportReimport && processed[norm.NFC.String(dirPath)] {
			continue
		}
		album, ok, err := source.QuickScanDir(dirPath, d.name, scanCache, d.mtime)
		if err != nil || !ok {
			continue
		}
		candidates = append(candidates, album)
	}
	if scanCache != nil {
		scanCache.Save() //nolint:errcheck
	}
	stop()

	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "No unprocessed albums found.")
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s %s\n\n",
		styleFound.Render("✓"),
		styleLabel.Render(fmt.Sprintf("%d unprocessed albums", len(candidates))),
	)

	p := tea.NewProgram(picker.New(candidates))
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("picker: %w", err)
	}
	m := finalModel.(picker.Model)
	if !m.Confirmed {
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return nil
	}
	selected := m.Selected()
	if len(selected) == 0 {
		fmt.Fprintln(os.Stderr, "Nothing selected.")
		return nil
	}

	fmt.Println()
	for i, item := range selected {
		fmt.Printf("==> [%d/%d] %s\n", i+1, len(selected), item.Name)
		if err := runBeetImport(importArgs(item.Path)...); err != nil {
			return fmt.Errorf("import stopped: %w", err)
		}
		exec.Command("pkill", "-TERM", "fpcalc").Run() //nolint:errcheck
	}
	return nil
}

type dirInfo struct {
	name  string
	mtime time.Time
}

func listDirsByMtime(root string) ([]dirInfo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read source dir: %w", err)
	}
	var dirs []dirInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "ignored" || name == "torrents" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		dirs = append(dirs, dirInfo{name: name, mtime: info.ModTime()})
	}
	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].mtime.After(dirs[j].mtime)
	})
	return dirs, nil
}

func importArgs(path string) []string {
	if flagImportReimport {
		return []string{"--noincremental", path}
	}
	return []string{path}
}

// readPathsFromFile reads album paths from r.
// If filename ends in ".csv" it parses the file as CSV and extracts the
// "source_path" column. Otherwise every non-empty, non-comment line is
// treated as a raw path (the original plain-text format).
func readPathsFromFile(r io.Reader) ([]string, error) {
	return readPathsFromFileNamed(r, flagImportFile)
}

func readPathsFromFileNamed(r io.Reader, filename string) ([]string, error) {
	buf := new(strings.Builder)
	if _, err := io.Copy(buf, r); err != nil {
		return nil, err
	}
	content := buf.String()

	if strings.ToLower(filepath.Ext(filename)) == ".csv" {
		cr := csv.NewReader(strings.NewReader(content))
		header, err := cr.Read()
		if err != nil {
			return nil, fmt.Errorf("reading CSV header: %w", err)
		}
		colIdx := -1
		for i, h := range header {
			if strings.TrimSpace(h) == "source_path" {
				colIdx = i
				break
			}
		}
		if colIdx < 0 {
			return nil, fmt.Errorf("CSV file has no 'source_path' column")
		}
		var paths []string
		for {
			row, err := cr.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("reading CSV row: %w", err)
			}
			if colIdx < len(row) {
				if p := strings.TrimSpace(row[colIdx]); p != "" {
					paths = append(paths, p)
				}
			}
		}
		return paths, nil
	}

	// Plain text: one path per line, skip empty lines and comments.
	var paths []string
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		paths = append(paths, line)
	}
	return paths, sc.Err()
}

func runImportFromFile() error {
	if err := requireFlag("beet", flagBeet); err != nil {
		return err
	}

	f, err := os.Open(flagImportFile)
	if err != nil {
		return fmt.Errorf("open %s: %w", flagImportFile, err)
	}
	defer f.Close()

	paths, err := readPathsFromFile(f)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "No paths found in file.")
		return nil
	}

	if flagImportLimit > 0 && len(paths) > flagImportLimit {
		paths = paths[:flagImportLimit]
	}

	for i, path := range paths {
		fmt.Printf("==> [%d/%d] %s\n", i+1, len(paths), filepath.Base(path))
		if err := runBeetImport(importArgs(path)...); err != nil {
			return fmt.Errorf("import stopped: %w", err)
		}
		exec.Command("pkill", "-TERM", "fpcalc").Run() //nolint:errcheck
	}
	return nil
}

// readAlbumIDs reads one beets album ID per line. Empty lines and lines
// starting with "#" are skipped, like in path lists.
func readAlbumIDs(r io.Reader) ([]int, error) {
	var ids []int
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		id, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("not an album ID: %q", line)
		}
		ids = append(ids, id)
	}
	return ids, sc.Err()
}

func runRetagFromFile() error {
	if flagImportFile == "" {
		return fmt.Errorf("--library needs --from-file with a list of album IDs")
	}
	if err := requireFlag("db", flagDB); err != nil {
		return err
	}
	if err := requireFlag("beet", flagBeet); err != nil {
		return err
	}

	f, err := os.Open(flagImportFile)
	if err != nil {
		return fmt.Errorf("open %s: %w", flagImportFile, err)
	}
	defer f.Close()

	ids, err := readAlbumIDs(f)
	if err != nil {
		return fmt.Errorf("%s: %w", flagImportFile, err)
	}
	albums, err := beets.UnmarkedAlbums(flagDB, retagField, ids)
	if err != nil {
		return err
	}
	if len(albums) == 0 {
		fmt.Fprintf(os.Stderr, "All %d albums in the list are retagged.\n", len(ids))
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s %s\n\n",
		styleFound.Render("✓"),
		styleLabel.Render(fmt.Sprintf("%d of %d albums left to retag", len(albums), len(ids))),
	)

	if flagImportLimit > 0 && len(albums) > flagImportLimit {
		albums = albums[:flagImportLimit]
	}

	mark := retagField + "=" + time.Now().Format("2006-01-02")
	for i, a := range albums {
		fmt.Printf("==> [%d/%d] %s - %s (id %d)\n", i+1, len(albums), a.AlbumArtist, a.Album, a.ID)
		if err := runBeetImport("-L", "--set", mark, fmt.Sprintf("id:%d", a.ID)); err != nil {
			return fmt.Errorf("import stopped: %w", err)
		}
		exec.Command("pkill", "-TERM", "fpcalc").Run() //nolint:errcheck
	}
	return nil
}
