package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
	"github.com/dewey/beets-importer/internal/doctor/linters"
	"github.com/dewey/beets-importer/internal/picker"
	"github.com/spf13/cobra"
)

var (
	flagMaintenanceFolders bool
	flagMaintenanceLimit   int
)

var maintenanceCmd = &cobra.Command{
	Use:   "maintenance",
	Short: "Run the doctor linters, pick what to fix, and work through it as a queue",
	Long: `Run the doctor linters and list each one with its number of cases.
Pick linters with SPACE, then pick the cases to fix, and beets-importer works
through them one by one: retag with 'beet import -L', fetch artwork, import
untracked folders, or remove empty folders.

The linters that walk the library folder are slow over a network share, so
they only run with --folders.`,
	RunE: runMaintenance,
}

func init() {
	addDBFlag(maintenanceCmd)
	addSourceFlag(maintenanceCmd)
	addBeetFlag(maintenanceCmd)
	maintenanceCmd.Flags().BoolVar(&flagMaintenanceFolders, "folders", false,
		"Also run the linters that walk the library folder (empty_dirs, untracked_dirs, duplicate_names)")
	maintenanceCmd.Flags().IntVar(&flagMaintenanceLimit, "limit", 0,
		"Maximum number of fixes to run (0 = no limit)")
}

// folderLinters walk the library folder instead of reading the beets database.
var folderLinters = map[string]bool{"empty_dirs": true, "untracked_dirs": true, "duplicate_names": true}

// fix is what maintenance does for the cases of one linter.
type fix struct {
	label   string // shown in the linter list, e.g. "retag"
	byAlbum bool   // run once per album, not once per issue
	retag   bool   // hide albums that were already retagged or skipped
	deletes bool   // deletes files, so the queue asks once before it starts
	run     func(t task) error
}

// task is one queued fix.
type task struct {
	label   string
	deletes bool
	title   string
	path    string
	albumID int
	run     func(t task) error
}

func maintenanceFixes(date string) map[string]fix {
	// Retags have no run func: the queue hands runs of them to retagAlbums
	// as batches, so beets can look up ahead.
	retag := fix{label: "retag", byAlbum: true, retag: true}
	return map[string]fix{
		"split_albums":       retag,
		"artist_variants":    retag,
		"missing_year":       retag,
		"lowercase_metadata": retag,
		"split_imports": {label: "merge", byAlbum: true, run: func(t task) error {
			return mergeSplitImport(t.albumID, date)
		}},
		"protected_audio": {label: "delete", deletes: true, run: func(t task) error {
			return runBeet("remove", "-d", "-f", "path:"+t.path)
		}},
		"missing_artwork": {label: "fetch art", byAlbum: true, run: func(t task) error {
			return runBeet("fetchart", fmt.Sprintf("id:%d", t.albumID))
		}},
		// The folder is inside the library, so move it into place instead of
		// copying. It may have been imported and removed before, which the
		// incremental state would skip.
		"untracked_dirs": {label: "import", run: func(t task) error {
			return runBeetImport("-m", "--noincremental", t.path)
		}},
		"empty_dirs": {label: "remove folder", run: func(t task) error {
			return os.Remove(t.path)
		}},
	}
}

func runMaintenance(_ *cobra.Command, _ []string) error {
	if err := requireFlag("db", flagDB); err != nil {
		return err
	}
	if err := requireFlag("beet", flagBeet); err != nil {
		return err
	}
	var libraryRoot string
	if flagMaintenanceFolders {
		if err := requireFlag("source", flagSource); err != nil {
			return err
		}
		var err error
		libraryRoot, err = beets.LibraryDir(flagBeet)
		if err != nil {
			return err
		}
	}

	var spinMsg atomic.Value
	spinMsg.Store("Loading library…")
	stop := startSpinner(&spinMsg)
	albums, err := beets.LoadAlbums(flagDB)
	if err != nil {
		stop()
		return fmt.Errorf("loading albums: %w", err)
	}
	items, err := beets.LoadItems(flagDB)
	if err != nil {
		stop()
		return fmt.Errorf("loading items: %w", err)
	}

	all := buildSpecs(albums, items, libraryRoot, flagSource, loadedConfig)
	var specs []doctor.Spec
	for _, s := range all {
		if s.Enabled && (flagMaintenanceFolders || !folderLinters[s.Linter.Name()]) {
			specs = append(specs, s)
		}
	}
	spinMsg.Store("Running linters…")
	byName := make(map[string]doctor.Result)
	for r := range doctor.Run(context.Background(), specs) {
		byName[r.LinterName] = r
	}
	stop()

	if _, err := headlessIssues(specs, byName); err != nil {
		return err
	}

	date := time.Now().Format("2006-01-02")
	fixes := maintenanceFixes(date)
	retagged, err := beets.MarkedAlbums(flagDB, retagFields)
	if err != nil {
		return err
	}
	albumByID := make(map[int]beets.Album, len(albums))
	for _, a := range albums {
		albumByID[a.ID] = a
	}

	queues := make(map[string][]task)
	var linterItems []picker.Item
	selectable := 0
	for _, s := range all {
		name := s.Linter.Name()
		desc := s.Linter.Description()
		r, ran := byName[name]
		f, hasFix := fixes[name]
		item := picker.Item{Key: name, Disabled: true}
		switch {
		case !s.Enabled:
			item.Line1, item.Line2 = desc, "turned off in the config"
		case !ran:
			item.Line1, item.Line2 = desc, "not checked, walks the library folder; run with --folders"
		case !hasFix && len(r.Issues) > 0:
			item.Line1 = fmt.Sprintf("%s — %d issues", desc, len(r.Issues))
			item.Line2 = "report only, see 'doctor --linter " + name + "'"
		default:
			var q []task
			if hasFix {
				q = buildTasks(f, r.Issues, albumByID, retagged)
			}
			if len(q) == 0 {
				item.Line1 = desc + " — nothing to do"
				item.Done = true
				break
			}
			queues[name] = q
			item.Disabled = false
			item.Line1 = fmt.Sprintf("%s — %d %s", desc, len(q), unit(f))
			item.Line2 = f.label
			selectable++
		}
		linterItems = append(linterItems, item)
	}
	if selectable == 0 {
		// No picker needed, but still show that every check is fine.
		for _, it := range linterItems {
			mark := styleDim.Render(" - ")
			if it.Done {
				mark = styleFound.Render(" ✓ ")
			}
			fmt.Fprintln(os.Stderr, "  "+mark+" "+it.Line1)
			if it.Line2 != "" {
				fmt.Fprintln(os.Stderr, "      "+styleDim.Render(it.Line2))
			}
		}
		fmt.Fprintln(os.Stderr, "\nNothing to fix.")
		return nil
	}

	chosen, ok, err := pick("Select checks to fix", linterItems)
	if err != nil || !ok {
		return err
	}
	var queue []task
	seen := make(map[string]bool)
	for _, li := range chosen {
		q := queues[li.Key]
		list := make([]picker.Item, len(q))
		for i, t := range q {
			list[i] = picker.Item{Key: strconv.Itoa(i), Name: t.title, Path: t.path, Line1: t.title, Line2: t.path}
		}
		sel, ok, err := pick(fmt.Sprintf("Select cases to %s (%s)", fixes[li.Key].label, li.Key), list)
		if err != nil || !ok {
			return err
		}
		for _, it := range sel {
			i, err := strconv.Atoi(it.Key)
			if err != nil {
				return err
			}
			t := q[i]
			// An album found by two linters only needs one retag.
			k := fmt.Sprintf("%s|%d|%s", t.label, t.albumID, t.path)
			if !seen[k] {
				seen[k] = true
				queue = append(queue, t)
			}
		}
	}
	if len(queue) == 0 {
		fmt.Fprintln(os.Stderr, "Nothing selected.")
		return nil
	}
	if flagMaintenanceLimit > 0 && len(queue) > flagMaintenanceLimit {
		queue = queue[:flagMaintenanceLimit]
	}
	deletes := 0
	for _, t := range queue {
		if t.deletes {
			deletes++
		}
	}
	if deletes > 0 {
		ok, err := askYesNo(fmt.Sprintf("The queue deletes %d files from disk. Go ahead?", deletes))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(os.Stderr, "Cancelled.")
			return nil
		}
	}

	for i := 0; i < len(queue); i++ {
		t := queue[i]
		if t.run == nil {
			// Collect the run of retags that starts here into one call.
			titles := make(map[int]string)
			var ids []int
			for ; i < len(queue) && queue[i].run == nil; i++ {
				ids = append(ids, queue[i].albumID)
				titles[queue[i].albumID] = queue[i].title
			}
			i--
			if err := retagAlbums(ids, func(id int) string { return titles[id] }, date); err != nil {
				return fmt.Errorf("maintenance stopped: %w", err)
			}
			continue
		}
		id := ""
		if t.albumID != 0 {
			id = fmt.Sprintf(" (id %d)", t.albumID)
		}
		fmt.Printf("==> [%d/%d] %s: %s%s\n", i+1, len(queue), t.label, t.title, id)
		if err := t.run(t); err != nil {
			return fmt.Errorf("maintenance stopped: %w", err)
		}
		exec.Command("pkill", "-TERM", "fpcalc").Run() //nolint:errcheck
	}
	return nil
}

// buildTasks turns the issues of one linter into queued fixes. Album fixes
// get one task per album, named after the album, so track linters do not
// queue the same album for every track.
func buildTasks(f fix, issues []doctor.Issue, albums map[int]beets.Album, retagged map[int]bool) []task {
	var tasks []task
	index := make(map[int]int)
	counts := make(map[int]int)
	for _, iss := range issues {
		t := task{label: f.label, deletes: f.deletes, title: iss.Description, path: iss.Path, albumID: iss.AlbumID, run: f.run}
		if f.byAlbum {
			if iss.AlbumID == 0 || (f.retag && retagged[iss.AlbumID]) {
				continue
			}
			counts[iss.AlbumID]++
			if _, ok := index[iss.AlbumID]; ok {
				continue
			}
			if a, ok := albums[iss.AlbumID]; ok {
				t.path = a.Path
			}
			index[iss.AlbumID] = len(tasks)
		}
		tasks = append(tasks, t)
	}
	for id, i := range index {
		if n := counts[id]; n > 1 {
			a := albums[id]
			tasks[i].title = fmt.Sprintf("%s — %s: %d tracks", a.AlbumArtist, a.Album, n)
		}
	}
	return tasks
}

func unit(f fix) string {
	if f.byAlbum {
		return "albums"
	}
	return "folders"
}

// pick shows a picker and returns the selected items. ok is false when the
// user cancelled.
func pick(title string, items []picker.Item) ([]picker.Item, bool, error) {
	final, err := tea.NewProgram(picker.NewFromItems(title, items)).Run()
	if err != nil {
		return nil, false, fmt.Errorf("picker: %w", err)
	}
	m := final.(picker.Model)
	if !m.Confirmed {
		fmt.Fprintln(os.Stderr, "Cancelled.")
		return nil, false, nil
	}
	return m.Selected(), true, nil
}

// mergeSplitImport asks first, then puts the tracks of a split import into
// the album with the lowest ID, removes the empty album rows, and retags the
// result. The group
// is read again here, because earlier fixes in the queue may have changed it.
func mergeSplitImport(albumID int, date string) error {
	albums, err := beets.LoadAlbums(flagDB)
	if err != nil {
		return err
	}
	items, err := beets.LoadItems(flagDB)
	if err != nil {
		return err
	}
	var group []beets.Album
	for _, g := range linters.SplitImportGroups(albums, items) {
		for _, a := range g {
			if a.ID == albumID {
				group = g
			}
		}
	}
	if len(group) == 0 {
		fmt.Printf("Album %d is no longer split, nothing to merge.\n", albumID)
		return nil
	}

	keep := group[0].ID
	fmt.Printf("These %d albums look like one split import:\n", len(group))
	for _, a := range group {
		fmt.Printf("  id %-6d %2d tracks  %s — %s  (%s)\n", a.ID, a.TrackCount, a.AlbumArtist, a.Album, a.Path)
	}
	ok, err := askYesNo(fmt.Sprintf("Merge them into album %d?", keep))
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Not merged.")
		return nil
	}

	ids := make([]string, len(group))
	var others []int
	for i, a := range group {
		ids[i] = strconv.Itoa(a.ID)
		if a.ID != keep {
			others = append(others, a.ID)
		}
	}
	// -M and -W: only the database changes here. The retag below moves the
	// files and writes the tags.
	if err := runBeet("modify", "-y", "-M", "-W", "album_id::^("+strings.Join(ids, "|")+")$", fmt.Sprintf("album_id=%d", keep)); err != nil {
		return err
	}
	counts, err := beets.ItemCounts(flagDB, others)
	if err != nil {
		return err
	}
	if len(counts) > 0 {
		return fmt.Errorf("merge into album %d left tracks in other albums: %v", keep, counts)
	}
	if err := runBeet("remove", "-a", "-f", "id::^("+strings.Join(ids[1:], "|")+")$"); err != nil {
		return err
	}
	fmt.Printf("Merged %d albums into album %d.\n", len(group), keep)

	title := func(int) string { return group[0].AlbumArtist + " — " + group[0].Album }
	if err := retagAlbums([]int{keep}, title, date); err != nil {
		return err
	}
	// After a skip the album keeps its ID, and its tracks may still sit in
	// one folder per old album, so gather them.
	left, err := beets.UnmarkedAlbums(flagDB, []string{retagField}, []int{keep})
	if err != nil {
		return err
	}
	if len(left) > 0 {
		return runBeet("move", "-a", fmt.Sprintf("id:%d", keep))
	}
	return nil
}
