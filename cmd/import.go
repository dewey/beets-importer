package cmd

import (
	"bufio"
	"context"
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
	"github.com/dewey/beets-importer/internal/navidrome"
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
	flagImportPlaylist string
)

// retagField is set on every album retagged with --library or maintenance,
// and skipField on every album skipped at the beets prompt. beets keeps both,
// so later runs leave those albums out.
const (
	retagField = "retagged"
	skipField  = "retag_skipped"
)

var retagFields = []string{retagField, skipField}

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
		"Retag library albums with 'beet import -L'; album IDs come from --from-file or --from-playlist")
	importCmd.Flags().StringVar(&flagImportPlaylist, "from-playlist", "",
		"With --library, retag every album that has a track in this Navidrome playlist ID")
	importCmd.MarkFlagsMutuallyExclusive("from-file", "since")
	importCmd.MarkFlagsMutuallyExclusive("from-file", "from-playlist")
	importCmd.MarkFlagsMutuallyExclusive("library", "reimport")
}

func runImport(_ *cobra.Command, _ []string) error {
	if flagImportLibrary {
		return runRetag()
	}
	if flagImportPlaylist != "" {
		return fmt.Errorf("--from-playlist needs --library")
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
		if err != nil || !ok || loadedConfig.Ignore.Album(album.Album) {
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

func runRetag() error {
	if flagImportFile == "" && flagImportPlaylist == "" {
		return fmt.Errorf("--library needs --from-file with a list of album IDs, or --from-playlist")
	}
	if err := requireFlag("db", flagDB); err != nil {
		return err
	}
	if err := requireFlag("beet", flagBeet); err != nil {
		return err
	}

	var ids []int
	var err error
	if flagImportPlaylist != "" {
		ids, err = playlistAlbumIDs(context.Background(), flagImportPlaylist)
	} else {
		ids, err = fileAlbumIDs(flagImportFile)
	}
	if err != nil {
		return err
	}
	unmarked, err := beets.UnmarkedAlbums(flagDB, retagFields, ids)
	if err != nil {
		return err
	}
	var albums []beets.LibraryAlbum
	for _, a := range unmarked {
		if !loadedConfig.Ignore.Album(a.Album) {
			albums = append(albums, a)
		}
	}
	if len(albums) == 0 {
		fmt.Fprintf(os.Stderr, "Nothing left to retag: all %d albums in the list are retagged, ignored or no longer in the library.\n", len(ids))
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s %s\n\n",
		styleFound.Render("✓"),
		styleLabel.Render(fmt.Sprintf("%d left to retag (%d of %d already retagged, ignored or no longer in the library)",
			len(albums), len(ids)-len(albums), len(ids))),
	)

	if flagImportLimit > 0 && len(albums) > flagImportLimit {
		albums = albums[:flagImportLimit]
	}

	todo := make([]int, len(albums))
	byID := make(map[int]beets.LibraryAlbum, len(albums))
	for i, a := range albums {
		todo[i] = a.ID
		byID[a.ID] = a
	}
	title := func(id int) string { return byID[id].AlbumArtist + " - " + byID[id].Album }
	if err := retagAlbums(todo, title, time.Now().Format("2006-01-02")); err != nil {
		return fmt.Errorf("import stopped: %w", err)
	}
	return nil
}

func fileAlbumIDs(path string) ([]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	ids, err := readAlbumIDs(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return ids, nil
}

// playlistAlbumIDs maps the tracks of a Navidrome playlist to beets album IDs
// through their file paths. Tracks that beets does not know are listed, so a
// stale Navidrome scan is easy to spot.
func playlistAlbumIDs(ctx context.Context, playlistID string) ([]int, error) {
	nd := loadedConfig.Navidrome
	if nd.URL == "" || nd.Username == "" || nd.PasswordCommand == "" {
		return nil, fmt.Errorf("--from-playlist needs navidrome.url, navidrome.username and navidrome.password_command in %s", loadedConfigPath)
	}
	out, err := exec.CommandContext(ctx, "sh", "-c", nd.PasswordCommand).Output()
	if err != nil {
		return nil, fmt.Errorf("navidrome.password_command: %w", err)
	}
	client, err := navidrome.Login(ctx, strings.TrimRight(nd.URL, "/"), nd.Username, strings.TrimSpace(string(out)))
	if err != nil {
		return nil, err
	}
	rel, err := client.PlaylistPaths(ctx, playlistID)
	if err != nil {
		return nil, err
	}
	library, err := beets.LibraryDir(flagBeet)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(rel))
	for i, p := range rel {
		paths[i] = filepath.Join(library, p)
	}
	ids, missing, err := beets.AlbumIDsByPath(flagDB, paths)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "Playlist has %d tracks from %d albums.\n", len(paths), len(ids))
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "%d tracks are not in the beets library. If beets moved them, run a Navidrome scan:\n", len(missing))
		for _, p := range missing {
			fmt.Fprintf(os.Stderr, "  %s\n", p)
		}
		fmt.Fprintln(os.Stderr)
	}
	return ids, nil
}

// retagBatch is how many albums one 'beet import -L' call gets. beets looks
// up the next albums while you decide on the current one, and an aBort only
// ends the rest of one batch.
const retagBatch = 20

// retagAlbums retags the albums with 'beet import -L', retagBatch at a time.
// title gives the line shown for each album before its batch starts, so the
// album IDs are at hand, e.g. to delete one.
func retagAlbums(ids []int, title func(id int) string, date string) error {
	for start := 0; start < len(ids); start += retagBatch {
		batch := ids[start:min(start+retagBatch, len(ids))]
		fmt.Printf("==> [%d–%d/%d] retag\n", start+1, start+len(batch), len(ids))
		for _, id := range batch {
			fmt.Printf("    %s (id %d)\n", title(id), id)
		}
		if err := retagBatchOnce(batch, date); err != nil {
			return err
		}
		exec.Command("pkill", "-TERM", "fpcalc").Run() //nolint:errcheck
	}
	return nil
}

// retagBatchOnce runs one 'beet import -L' call. beets exits with 0 after
// Apply, Skip and aBort alike, so afterwards each album is sorted out on its
// own: a missing ID means applied, used as-is or deleted; a skip entry in the
// import log for one of its folders is remembered with skipField; anything
// else was never reached, after aBort or "Merge all" on a duplicate.
func retagBatchOnce(ids []int, date string) error {
	// Read the folders before the run, because an apply moves the files.
	folders, err := beets.AlbumFolders(flagDB, ids)
	if err != nil {
		return err
	}
	logFile, err := os.CreateTemp("", "beets-importer-*.log")
	if err != nil {
		return err
	}
	logFile.Close()
	defer os.Remove(logFile.Name())

	if err := runBeetImport("-L", "-l", logFile.Name(), "--set", retagField+"="+date, idQuery(ids)); err != nil {
		return err
	}
	log, err := os.ReadFile(logFile.Name())
	if err != nil {
		return err
	}
	left, err := beets.UnmarkedAlbums(flagDB, retagFields, ids)
	if err != nil {
		return err
	}
	skipped, untouched := sortOutBatch(left, folders, skippedFolders(string(log)))
	if len(skipped) > 0 {
		if err := runBeet("modify", "-a", "-y", "-M", "-W", idQuery(skipped), skipField+"="+date); err != nil {
			return err
		}
	}
	if len(untouched) == 0 {
		return nil
	}
	fmt.Printf("beets ended before it applied or skipped %d of these albums (aBort, or a duplicate choice like Merge all). They were not changed: %v\n", len(untouched), untouched)
	ok, err := askYesNo("Continue?")
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("stopped, %d albums of this batch were not changed", len(untouched))
	}
	return nil
}

func idQuery(ids []int) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = strconv.Itoa(id)
	}
	return "id::^(" + strings.Join(s, "|") + ")$"
}

// skippedFolders returns the folders of the skip entries in a beets import
// log. An entry lists several folders, separated by "; ", for a multi-disc
// album.
func skippedFolders(log string) map[string]bool {
	skipped := make(map[string]bool)
	for _, line := range strings.Split(log, "\n") {
		verb, rest, ok := strings.Cut(line, " ")
		if !ok || (verb != "skip" && verb != "duplicate-skip") {
			continue
		}
		for _, p := range strings.Split(rest, "; ") {
			skipped[norm.NFC.String(filepath.Clean(p))] = true
		}
	}
	return skipped
}

// sortOutBatch splits the albums that are still there after a batch into
// those beets skipped and those it never reached.
func sortOutBatch(left []beets.LibraryAlbum, folders map[int][]string, skipped map[string]bool) (skippedIDs, untouched []int) {
	for _, a := range left {
		hit := false
		for _, f := range folders[a.ID] {
			if skipped[f] {
				hit = true
			}
		}
		if hit {
			skippedIDs = append(skippedIDs, a.ID)
		} else {
			untouched = append(untouched, a.ID)
		}
	}
	return skippedIDs, untouched
}

// askYesNo asks on the terminal, like beets does, and needs a clear y or n.
func askYesNo(question string) (bool, error) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		return false, err
	}
	defer tty.Close()
	r := bufio.NewReader(tty)
	for {
		fmt.Printf("%s (y/n) ", question)
		line, err := r.ReadString('\n')
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
	}
}
