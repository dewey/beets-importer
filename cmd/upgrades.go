package cmd

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/compare"
	"github.com/dewey/beets-importer/internal/matcher"
	"github.com/dewey/beets-importer/internal/picker"
	"github.com/dewey/beets-importer/internal/source"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/spf13/cobra"
)

var (
	flagUpgradesThreshold        float64
	flagUpgradesMinBitrateDelta  int
	flagUpgradesAll              bool
	flagUpgradesLimit            int
	flagUpgradesOutput           string
	flagUpgradesRequireYearMatch bool
	flagUpgradesLibraryFormat    []string
	flagUpgradesSourceFormat     []string
	flagUpgradesInteractive      bool
)

var upgradesCmd = &cobra.Command{
	Use:   "upgrades",
	Short: "Find albums in the source directory that would upgrade your library",
	RunE:  runUpgrades,
}

func init() {
	addDBFlag(upgradesCmd)
	addSourceFlag(upgradesCmd)
	addBeetFlag(upgradesCmd)
	addScanFlags(upgradesCmd)
	upgradesCmd.Flags().Float64Var(&flagUpgradesThreshold, "threshold", 0.70,
		"Minimum match confidence (0..1) to consider a source→library pair")
	upgradesCmd.Flags().IntVar(&flagUpgradesMinBitrateDelta, "min-bitrate-delta", 32,
		"Minimum bitrate improvement in kbps to flag as an upgrade (same-format comparisons)")
	upgradesCmd.Flags().BoolVar(&flagUpgradesAll, "all", false,
		"Show all matched pairs, not just upgrade candidates")
	upgradesCmd.Flags().IntVar(&flagUpgradesLimit, "limit", 0,
		"Stop after collecting this many upgrade candidates (0 = no limit)")
	upgradesCmd.Flags().StringVarP(&flagUpgradesOutput, "output", "o", "",
		"Write source paths to this file instead of printing a table (pass to beets-importer import --from-file)")
	upgradesCmd.Flags().BoolVar(&flagUpgradesRequireYearMatch, "require-year-match", false,
		"Skip candidates where both source and library have a known year that differs")
	upgradesCmd.Flags().StringSliceVar(&flagUpgradesLibraryFormat, "library-format", nil,
		"Only show candidates where the library copy is one of these formats (e.g. MP3,AAC)")
	upgradesCmd.Flags().StringSliceVar(&flagUpgradesSourceFormat, "source-format", nil,
		"Only show candidates where the source copy is one of these formats (e.g. FLAC)")
	upgradesCmd.Flags().BoolVarP(&flagUpgradesInteractive, "interactive", "i", false,
		"After scanning, show an interactive picker to select candidates and import them")
}

// ── Bubble Tea messages ───────────────────────────────────────────────────────

type libraryLoadedMsg struct {
	albums []beets.Album
	err    error
}

type albumScannedMsg struct {
	album source.Album
	done  int
	total int
}

type scanDoneMsg struct{ err error }

type warnMsg struct {
	dirName string
	err     error
}

// ── Model ─────────────────────────────────────────────────────────────────────

type upgradesPhase int

const (
	phaseLoading upgradesPhase = iota
	phaseScanning
	phaseDone
)

type upgradesModel struct {
	spinner spinner.Model
	phase   upgradesPhase

	libraryAlbums []beets.Album
	scanDone      int
	scanTotal     int
	currentDir    string
	candidates    []compare.Candidate
	warnings      []string
	err           error
	cancelled     bool

	cancelScan context.CancelFunc
	scanCh     chan tea.Msg

	// config (captured from flags at construction time)
	threshold        float64
	minBitrate       int
	verbose          bool
	limit            int
	requireYearMatch bool
	libraryFormats   []string // upper-cased; empty = no filter
	sourceFormats    []string // upper-cased; empty = no filter
	scanCache        *source.ScanCache
}

func newUpgradesModel(scanCache *source.ScanCache) upgradesModel {
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: spinnerFrames,
		FPS:    time.Second / 12,
	}
	return upgradesModel{
		spinner:          s,
		phase:            phaseLoading,
		threshold:        flagUpgradesThreshold,
		minBitrate:       flagUpgradesMinBitrateDelta,
		verbose:          flagUpgradesAll,
		limit:            flagUpgradesLimit,
		requireYearMatch: flagUpgradesRequireYearMatch,
		libraryFormats:   upperAll(flagUpgradesLibraryFormat),
		sourceFormats:    upperAll(flagUpgradesSourceFormat),
		scanCache:        scanCache,
	}
}

func (m upgradesModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			albums, err := beets.LoadAlbums(flagDB)
			return libraryLoadedMsg{albums: albums, err: err}
		},
	)
}

func (m upgradesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			if m.cancelScan != nil {
				m.cancelScan()
			}
			m.cancelled = true
			m.phase = phaseDone
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case libraryLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.phase = phaseDone
			return m, tea.Quit
		}
		for _, a := range msg.albums {
			if !loadedConfig.Ignore.Album(a.Album) {
				m.libraryAlbums = append(m.libraryAlbums, a)
			}
		}
		m.phase = phaseScanning

		ctx, cancel := context.WithCancel(context.Background())
		m.cancelScan = cancel
		m.scanCh = make(chan tea.Msg, 256)
		go func() {
			err := source.ScanEach(ctx, flagSource, source.ScanOptions{
				Bitrate: true,
				Cache:   m.scanCache,
				OnWarn: func(dirName string, err error) {
					select {
					case <-ctx.Done():
					case m.scanCh <- warnMsg{dirName: dirName, err: err}:
					}
				},
			},
				func(a source.Album, done, total int) bool {
					select {
					case <-ctx.Done():
						return false
					case m.scanCh <- albumScannedMsg{album: a, done: done, total: total}:
						return true
					}
				})
			m.scanCh <- scanDoneMsg{err: err}
		}()
		return m, readScanCh(m.scanCh)

	case albumScannedMsg:
		m.scanDone = msg.done
		m.scanTotal = msg.total
		m.currentDir = msg.album.DirName
		if loadedConfig.Ignore.Album(msg.album.Album) {
			return m, readScanCh(m.scanCh)
		}

		matches := matcher.FindMatches([]source.Album{msg.album}, m.libraryAlbums, m.threshold)
		for _, match := range matches {
			if cand, ok := compare.Evaluate(match.Source, match.Library, match.Score, m.minBitrate, m.requireYearMatch); ok {
				if !formatAllowed(cand.Library.Format, m.libraryFormats) {
					continue
				}
				if !formatAllowed(cand.Source.Format, m.sourceFormats) {
					continue
				}
				m.candidates = append(m.candidates, cand)
			} else if m.verbose {
				m.candidates = append(m.candidates, compare.Candidate{
					Source:  match.Source,
					Library: match.Library,
					Score:   match.Score,
					Reasons: []string{"(no upgrade)"},
				})
			}
		}
		if m.limit > 0 && len(m.candidates) >= m.limit {
			m.cancelScan()
			m.phase = phaseDone
			return m, tea.Quit
		}
		return m, readScanCh(m.scanCh)

	case warnMsg:
		m.warnings = append(m.warnings, fmt.Sprintf("skipping %s: %v", msg.dirName, msg.err))
		return m, readScanCh(m.scanCh)

	case scanDoneMsg:
		if msg.err != nil {
			m.err = msg.err
		}
		m.phase = phaseDone
		return m, tea.Quit
	}
	return m, nil
}

func (m upgradesModel) View() tea.View {
	return tea.NewView(m.render())
}

func (m upgradesModel) render() string {
	spin := styleSpinner.Render(m.spinner.View())
	switch m.phase {
	case phaseLoading:
		return fmt.Sprintf("%s %s", spin, styleLabel.Render("Loading beets library…"))
	case phaseScanning:
		if m.scanTotal == 0 {
			return fmt.Sprintf("%s %s", spin, styleLabel.Render("Scanning source directory…"))
		}
		return fmt.Sprintf("%s %s %s %s",
			spin,
			styleDim.Render(fmt.Sprintf("%d/%d", m.scanDone, m.scanTotal)),
			styleLabel.Render("—"),
			styleDim.Render(truncate(m.currentDir, 50)),
		)
	default:
		// Return empty so Bubble Tea clears the spinner line on exit.
		return ""
	}
}

func readScanCh(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// upperAll returns a new slice with every element converted to upper-case.
func upperAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = strings.ToUpper(s)
	}
	return out
}

// formatAllowed reports whether format passes the allowlist filter.
// An empty allowlist means "no filter" (everything passes).
func formatAllowed(format string, allowlist []string) bool {
	if len(allowlist) == 0 {
		return true
	}
	f := strings.ToUpper(format)
	for _, a := range allowlist {
		if f == strings.ToUpper(a) {
			return true
		}
	}
	return false
}

// ── Command entry point ───────────────────────────────────────────────────────

func runUpgrades(_ *cobra.Command, _ []string) error {
	if err := requireFlag("db", flagDB); err != nil {
		return err
	}
	if err := requireFlag("source", flagSource); err != nil {
		return err
	}

	var scanCache *source.ScanCache
	if !flagNoCache {
		if cachePath, err := source.DefaultCachePath(); err == nil {
			scanCache, _ = source.LoadCache(cachePath)
		}
	}
	p := tea.NewProgram(newUpgradesModel(scanCache), tea.WithOutput(os.Stderr))
	final, err := p.Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	result := final.(upgradesModel)

	if result.cancelled {
		return nil
	}
	if result.err != nil {
		return result.err
	}

	// Print warnings collected during the TUI scan.
	if n := len(result.warnings); n > 0 {
		if flagVerbose {
			for _, w := range result.warnings {
				fmt.Fprintf(os.Stderr, "%s\n", styleDim.Render("warning: "+w))
			}
		} else {
			fmt.Fprintf(os.Stderr, "%s\n", styleDim.Render(
				fmt.Sprintf("skipped %d director%s that couldn't be scanned (run with --verbose to see details)", n, map[bool]string{true: "y", false: "ies"}[n == 1]),
			))
		}
	}

	fmt.Fprintf(os.Stderr, "%s %s\n",
		styleFound.Render("✓"),
		styleLabel.Render(fmt.Sprintf("%d albums loaded from library", len(result.libraryAlbums))),
	)
	fmt.Fprintf(os.Stderr, "%s %s\n\n",
		styleFound.Render("✓"),
		styleLabel.Render(fmt.Sprintf("%d upgrade candidate(s) found", len(result.candidates))),
	)

	if len(result.candidates) == 0 {
		fmt.Println("No upgrade candidates found.")
		return nil
	}

	// --output: write a CSV with all candidate details
	if flagUpgradesOutput != "" {
		f, err := os.Create(flagUpgradesOutput)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		w := csv.NewWriter(f)
		_ = w.Write([]string{
			"source_path", "source_dir", "source_year",
			"library_artist", "library_album", "library_year",
			"score", "format", "avg_bitrate_kbps", "upgrade_reason", "year_matches",
		})
		for _, c := range result.candidates {
			srcYear := ""
			if c.Source.Year > 0 {
				srcYear = strconv.Itoa(c.Source.Year)
			}
			libYear := ""
			if c.Library.Year > 0 {
				libYear = strconv.Itoa(c.Library.Year)
			}
			bitrateKbps := ""
			if c.Source.AvgBitrate > 0 && c.Library.AvgBitrate > 0 {
				bitrateKbps = fmt.Sprintf("%d→%d", c.Library.AvgBitrate/1000, c.Source.AvgBitrate/1000)
			}
			_ = w.Write([]string{
				c.Source.Path,
				c.Source.DirName,
				srcYear,
				c.Library.AlbumArtist,
				c.Library.Album,
				libYear,
				fmt.Sprintf("%.2f", c.Score),
				fmt.Sprintf("%s→%s", c.Library.Format, c.Source.Format),
				bitrateKbps,
				strings.Join(c.Reasons, "; "),
				strconv.FormatBool(c.YearMatches),
			})
		}
		w.Flush()
		if err := w.Error(); err != nil {
			return fmt.Errorf("writing CSV: %w", err)
		}
		fmt.Fprintf(os.Stderr, "%s Written %d candidates to %s\n",
			styleFound.Render("✓"), len(result.candidates), flagUpgradesOutput)
		return nil
	}

	// The picker shows its own summary, so skip the table.
	if !flagUpgradesInteractive {
		fmt.Println(styleHeader.Render("Upgrade Candidates"))
		fmt.Println(strings.Repeat("─", 100))

		table := tablewriter.NewTable(os.Stdout,
			tablewriter.WithHeader([]string{"Source Directory", "Library Match", "Score", "Year", "Format", "Avg Bitrate", "Upgrade Reason"}),
			tablewriter.WithHeaderAutoFormat(tw.Off),
		)
		for _, c := range result.candidates {
			libMatch := fmt.Sprintf("%s / %s", c.Library.AlbumArtist, c.Library.Album)
			if c.Library.Year > 0 {
				libMatch += fmt.Sprintf(" (%d)", c.Library.Year)
			}
			srcBitrate := ""
			if c.Source.AvgBitrate > 0 {
				srcBitrate = fmt.Sprintf("%d→%d kbps", c.Library.AvgBitrate/1000, c.Source.AvgBitrate/1000)
			}
			fmtChange := ""
			if c.Source.Format != "" {
				fmtChange = fmt.Sprintf("%s→%s", c.Library.Format, c.Source.Format)
			}
			var yearGlyph string
			switch {
			case c.YearMatches:
				yearGlyph = "✓"
			case c.Source.Year > 0 && c.Library.Year > 0:
				yearGlyph = "✗"
			default:
				yearGlyph = "?"
			}
			table.Append([]string{
				truncate(c.Source.DirName, 45),
				truncate(libMatch, 40),
				fmt.Sprintf("%.2f", c.Score),
				yearGlyph,
				fmtChange,
				srcBitrate,
				strings.Join(c.Reasons, "; "),
			})
		}
		table.Render()

		if flagUpgradesLimit > 0 && len(result.candidates) >= flagUpgradesLimit {
			fmt.Printf("\n(stopped after %d candidates — re-run without --limit to scan everything)\n", flagUpgradesLimit)
		}
	}

	if flagUpgradesInteractive {
		items := make([]picker.Item, len(result.candidates))
		for i, c := range result.candidates {
			items[i] = buildPickerItem(c)
		}
		pp := tea.NewProgram(picker.NewFromItems("Select albums to import", items))
		finalModel, err := pp.Run()
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
		fmt.Fprintf(os.Stderr, "\n%s\n", styleHeader.Render(fmt.Sprintf("Importing %d album(s)", len(selected))))
		fmt.Fprintln(os.Stderr, styleDim.Render(strings.Repeat("─", 90)))
		for i, item := range selected {
			idx := styleDim.Render(fmt.Sprintf("%2d.", i+1))
			fmt.Fprintf(os.Stderr, "%s %s\n", idx, item.Line1)
			fmt.Fprintf(os.Stderr, "    %s\n", item.Line2)
		}
		fmt.Fprintln(os.Stderr, styleDim.Render(strings.Repeat("─", 90)))
		fmt.Fprintln(os.Stderr)
		for i, item := range selected {
			fmt.Printf("==> [%d/%d] %s\n", i+1, len(selected), item.Name)
			if err := runBeetImport(item.Path); err != nil {
				return fmt.Errorf("import stopped: %w", err)
			}
		}
		return nil
	}

	return nil
}

// pickerStyles holds the lipgloss styles used when rendering upgrade candidates in the picker.
var pickerStyles = struct {
	label   lipgloss.Style
	match   lipgloss.Style
	warn    lipgloss.Style
	dim     lipgloss.Style
	upgrade lipgloss.Style
}{
	label:   lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
	match:   lipgloss.NewStyle().Foreground(lipgloss.Color("82")),
	warn:    lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	dim:     lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
	upgrade: lipgloss.NewStyle().Foreground(lipgloss.Color("51")),
}

// Column widths for the picker two-line table layout.
// pickerColLabel must fit the longest label ("Library:") = 8 chars.
const (
	pickerColLabel  = 8
	pickerColArtist = 18
	pickerColAlbum  = 22
	pickerColYear   = 4
	pickerColTracks = 3
	pickerColFormat = 5
)

// buildPickerItem builds a styled picker.Item for an upgrade candidate.
// Line1 shows the source (incoming) album; Line2 shows the matched library album.
// Fields that match between source and library are highlighted green; year
// mismatches are highlighted orange. The source format is shown in cyan to
// indicate the upgrade.
func buildPickerItem(c compare.Candidate) picker.Item {
	s := pickerStyles

	srcArtist := padRight(truncate(c.Source.Artist, pickerColArtist), pickerColArtist)
	libArtist := padRight(truncate(c.Library.AlbumArtist, pickerColArtist), pickerColArtist)
	artistMatch := strings.EqualFold(c.Source.Artist, c.Library.AlbumArtist)

	srcAlbum := padRight(truncate(c.Source.Album, pickerColAlbum), pickerColAlbum)
	libAlbum := padRight(truncate(c.Library.Album, pickerColAlbum), pickerColAlbum)
	albumMatch := strings.EqualFold(c.Source.Album, c.Library.Album)

	srcYearStr := "----"
	if c.Source.Year > 0 {
		srcYearStr = strconv.Itoa(c.Source.Year)
	}
	libYearStr := "----"
	if c.Library.Year > 0 {
		libYearStr = strconv.Itoa(c.Library.Year)
	}
	srcYearStr = padRight(srcYearStr, pickerColYear)
	libYearStr = padRight(libYearStr, pickerColYear)
	yearMatch := c.Source.Year > 0 && c.Library.Year > 0 && c.Source.Year == c.Library.Year
	yearMismatch := c.Source.Year > 0 && c.Library.Year > 0 && c.Source.Year != c.Library.Year

	srcTracksStr := padRight(strconv.Itoa(c.Source.TrackCount), pickerColTracks)
	libTracksStr := padRight(strconv.Itoa(c.Library.TrackCount), pickerColTracks)
	tracksMatch := c.Source.TrackCount > 0 && c.Library.TrackCount > 0 && c.Source.TrackCount == c.Library.TrackCount
	tracksMismatch := c.Source.TrackCount > 0 && c.Library.TrackCount > 0 && c.Source.TrackCount != c.Library.TrackCount

	fieldStyle := func(matches, mismatch bool) lipgloss.Style {
		switch {
		case mismatch:
			return s.warn
		case matches:
			return s.match
		default:
			return s.dim
		}
	}

	yearStyle := fieldStyle(yearMatch, yearMismatch)
	artistStyle := fieldStyle(artistMatch, false)
	albumStyle := fieldStyle(albumMatch, false)

	srcFmt := padRight(c.Source.Format, pickerColFormat)
	libFmt := padRight(c.Library.Format, pickerColFormat)

	tracksStyle := fieldStyle(tracksMatch, tracksMismatch)

	line1 := fmt.Sprintf("%s  %s  %s  %s  %s  %s  Similarity %.2f",
		s.label.Render(padRight("Source:", pickerColLabel)),
		artistStyle.Render(srcArtist),
		albumStyle.Render(srcAlbum),
		yearStyle.Render(srcYearStr),
		tracksStyle.Render(srcTracksStr),
		s.upgrade.Render(srcFmt),
		c.Score,
	)
	line2 := fmt.Sprintf("%s  %s  %s  %s  %s  %s  %s",
		s.label.Render(padRight("Library:", pickerColLabel)),
		artistStyle.Render(libArtist),
		albumStyle.Render(libAlbum),
		yearStyle.Render(libYearStr),
		tracksStyle.Render(libTracksStr),
		s.dim.Render(libFmt),
		s.dim.Render(strings.Join(c.Reasons, "; ")),
	)

	return picker.Item{
		Name:        c.Source.DirName,
		Path:        c.Source.Path,
		LibraryPath: c.Library.Path,
		Line1:       line1,
		Line2:       line2,
		Styled:      true,
	}
}
