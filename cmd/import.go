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
	"strings"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/matcher"
	"github.com/dewey/beets-importer/internal/picker"
	"github.com/dewey/beets-importer/internal/source"
	"github.com/spf13/cobra"
)

var (
	flagImportFile      string
	flagImportLatest    bool
	flagImportLimit     int
	flagImportAfter     string
	flagImportThreshold float64
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import albums from a file of paths or from the latest unimported albums",
	RunE:  runImport,
}

func init() {
	importCmd.Flags().StringVar(&flagImportFile, "from-file", "",
		"File containing one album path per line")
	importCmd.Flags().BoolVar(&flagImportLatest, "latest", false,
		"Pick from recently added unimported albums")
	importCmd.Flags().IntVar(&flagImportLimit, "limit", 0,
		"Maximum number of albums to process (0 = no limit)")
	importCmd.Flags().StringVar(&flagImportAfter, "after", "",
		"Only consider albums added after this date (YYYY-MM-DD); only used with --latest")
	importCmd.Flags().Float64Var(&flagImportThreshold, "threshold", 0.85,
		"Match confidence above which an album is considered already imported; only used with --latest")
}

func runImport(_ *cobra.Command, _ []string) error {
	switch {
	case flagImportLatest && flagImportFile != "":
		return fmt.Errorf("--latest and --from-file are mutually exclusive")
	case !flagImportLatest && flagImportFile == "":
		return fmt.Errorf("one of --latest or --from-file is required")
	case flagImportLatest:
		return runImportLatest()
	default:
		return runImportFromFile()
	}
}

func runImportLatest() error {
	if err := requireFlag("db", flagDB); err != nil {
		return err
	}
	if err := requireFlag("source", flagSource); err != nil {
		return err
	}
	if err := requireFlag("beet", flagBeet); err != nil {
		return err
	}

	var afterTime time.Time
	if flagImportAfter != "" {
		t, err := time.Parse("2006-01-02", flagImportAfter)
		if err != nil {
			return fmt.Errorf("--after: expected YYYY-MM-DD, got %q", flagImportAfter)
		}
		afterTime = t
	}

	var spinMsg atomic.Value

	spinMsg.Store("Loading beets library…")
	stop := startSpinner(&spinMsg)
	libraryAlbums, err := beets.LoadAlbums(flagDB)
	stop()
	if err != nil {
		return fmt.Errorf("load beets library: %w", err)
	}
	fmt.Fprintf(os.Stderr, "%s %s\n",
		styleFound.Render("✓"),
		styleLabel.Render(fmt.Sprintf("%d albums in library", len(libraryAlbums))),
	)

	logEntries := readImportLog(flagLog)

	spinMsg.Store("Listing source directory…")
	stop = startSpinner(&spinMsg)
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
		if !afterTime.IsZero() && d.mtime.Before(afterTime) {
			continue
		}
		if inLog(d.name, logEntries) {
			continue
		}
		dirPath := filepath.Join(flagSource, d.name)
		album, ok, err := source.QuickScanDir(dirPath, d.name, scanCache, d.mtime)
		if err != nil || !ok {
			continue
		}
		if matches := matcher.FindMatches([]source.Album{album}, libraryAlbums, flagImportThreshold); len(matches) > 0 {
			continue
		}
		candidates = append(candidates, album)
	}
	if scanCache != nil {
		scanCache.Save() //nolint:errcheck
	}
	stop()

	if len(candidates) == 0 {
		fmt.Fprintln(os.Stderr, "No unimported albums found.")
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s %s\n\n",
		styleFound.Render("✓"),
		styleLabel.Render(fmt.Sprintf("%d unimported albums", len(candidates))),
	)

	p := tea.NewProgram(picker.New(candidates), tea.WithAltScreen())
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
		if err := runBeetImport(item.Path); err != nil {
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

func readImportLog(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

func inLog(dirName string, logLines []string) bool {
	needle := "/" + dirName
	for _, line := range logLines {
		if strings.Contains(line, needle) {
			return true
		}
	}
	return false
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
		if err := runBeetImport(path); err != nil {
			return fmt.Errorf("import stopped: %w", err)
		}
		exec.Command("pkill", "-TERM", "fpcalc").Run() //nolint:errcheck
	}
	return nil
}
