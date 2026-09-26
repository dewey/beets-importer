package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/config"
	"github.com/dewey/beets-importer/internal/doctor"
	"github.com/dewey/beets-importer/internal/doctor/linters"
	"github.com/spf13/cobra"
)

var (
	flagDoctorJSON    bool
	flagDoctorLinters []string
	flagDoctorPaths   bool
	flagDoctorPrint0  bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the health of your beets library",
	Long: `Run a suite of linters against your beets library and report issues.

Checks include empty directories, untracked files, low-quality audio,
missing metadata, and more. Results are shown in an interactive scrollable view.
The library folder on disk is read from 'beet config'.`,
	RunE: runDoctor,
}

func init() {
	addDBFlag(doctorCmd)
	addBeetFlag(doctorCmd)
	doctorCmd.Flags().BoolVar(&flagDoctorJSON, "json", false,
		"Print results as JSON to stdout instead of the interactive view")
	doctorCmd.Flags().StringSliceVar(&flagDoctorLinters, "linter", nil,
		"Run only these linters, comma-separated (e.g. empty_dirs,missing_year)")
	doctorCmd.Flags().BoolVar(&flagDoctorPaths, "paths", false,
		"Print only the issue paths of the selected linters, one per line (requires --linter)")
	doctorCmd.Flags().BoolVarP(&flagDoctorPrint0, "print0", "0", false,
		"With --paths, separate paths with NUL instead of newline (for xargs -0)")
}

type doctorDataLoadedMsg struct {
	albums []beets.Album
	items  []beets.Item
	err    error
}

type doctorResultMsg struct {
	result doctor.Result
}

type doctorAllDoneMsg struct{}

type doctorPhase int

const (
	doctorPhaseLoading doctorPhase = iota
	doctorPhaseRunning
	doctorPhaseDone
)

type doctorModel struct {
	spinner  spinner.Model
	viewport viewport.Model
	phase    doctorPhase
	ready    bool // true once the viewport is initialised

	dbPath      string
	libraryRoot string
	cfg         config.DoctorConfig
	specs       []doctor.Spec

	totalLinters int
	doneLinters  int
	pending      map[string]bool // enabled linter names still in flight
	resultCh     <-chan doctor.Result

	results []doctor.Result

	width  int
	height int

	err error
}

func newDoctorModel(dbPath, libraryRoot string, cfg config.DoctorConfig) doctorModel {
	s := spinner.New()
	s.Spinner = spinner.Spinner{Frames: spinnerFrames, FPS: time.Second / 12}
	return doctorModel{
		spinner:     s,
		phase:       doctorPhaseLoading,
		dbPath:      dbPath,
		libraryRoot: libraryRoot,
		cfg:         cfg,
	}
}

func (m doctorModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg {
			albums, err := beets.LoadAlbums(m.dbPath)
			if err != nil {
				return doctorDataLoadedMsg{err: err}
			}
			items, err := beets.LoadItems(m.dbPath)
			return doctorDataLoadedMsg{albums: albums, items: items, err: err}
		},
	)
}

func (m doctorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.phase == doctorPhaseDone && m.ready {
			m.viewport.SetWidth(msg.Width)
			m.viewport.SetHeight(viewportHeight(msg.Height))
			m.viewport.SetContent(renderDoctorResults(m.results, m.specs, msg.Width))
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case doctorDataLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.phase = doctorPhaseDone
			return m, tea.Quit
		}
		m.specs, _ = selectSpecs(buildSpecs(msg.albums, msg.items, m.libraryRoot, m.cfg), flagDoctorLinters)
		m.totalLinters = len(m.specs)
		m.pending = make(map[string]bool)
		for _, s := range m.specs {
			if s.Enabled {
				m.pending[s.Linter.Name()] = true
			}
		}
		m.phase = doctorPhaseRunning
		m.resultCh = doctor.Run(context.Background(), m.specs)
		return m, readDoctorCh(m.resultCh)

	case doctorResultMsg:
		m.results = append(m.results, msg.result)
		m.doneLinters++
		delete(m.pending, msg.result.LinterName)
		if m.doneLinters == m.totalLinters {
			m.phase = doctorPhaseDone
			m.viewport = viewport.New(viewport.WithWidth(m.width), viewport.WithHeight(viewportHeight(m.height)))
			m.viewport.SetContent(renderDoctorResults(m.results, m.specs, m.width))
			m.ready = true
			return m, nil
		}
		return m, readDoctorCh(m.resultCh)

	case doctorAllDoneMsg:
		m.phase = doctorPhaseDone
		if !m.ready {
			m.viewport = viewport.New(viewport.WithWidth(m.width), viewport.WithHeight(viewportHeight(m.height)))
			m.viewport.SetContent(renderDoctorResults(m.results, m.specs, m.width))
			m.ready = true
		}
		return m, nil
	}

	if m.phase == doctorPhaseDone && m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

func readDoctorCh(ch <-chan doctor.Result) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-ch
		if !ok {
			return doctorAllDoneMsg{}
		}
		return doctorResultMsg{result: r}
	}
}

func viewportHeight(termHeight int) int {
	h := termHeight - 4 // header(1) + sep(1) + footer-sep(1) + footer(1)
	if h < 1 {
		h = 1
	}
	return h
}

var (
	styleDocHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleDocSep     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleDocEnabled = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	styleDocSkipped = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleDocError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	styleDocWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleDocIssue   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleDocSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	styleDocCount   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleDocKeys    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func (m doctorModel) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m doctorModel) render() string {
	spin := styleSpinner.Render(m.spinner.View())
	switch m.phase {
	case doctorPhaseLoading:
		return fmt.Sprintf("%s %s", spin, styleLabel.Render("Loading beets library…"))

	case doctorPhaseRunning:
		names := make([]string, 0, len(m.pending))
		for n := range m.pending {
			names = append(names, n)
		}
		sort.Strings(names)
		waiting := strings.Join(names, ", ")
		return fmt.Sprintf("%s %s",
			spin,
			styleLabel.Render(fmt.Sprintf("Running linters (%d/%d) — waiting for: %s", m.doneLinters, m.totalLinters, waiting)),
		)

	case doctorPhaseDone:
		if m.err != nil {
			return styleDocError.Render("Error: "+m.err.Error()) + "\n"
		}
		if !m.ready {
			return styleLabel.Render("Preparing results…")
		}
		w := m.width
		if w == 0 {
			w = 80
		}
		sep := styleDocSep.Render(strings.Repeat("─", w))
		header := styleDocHeader.Render("Beets Library Health Report")
		footer := renderDoctorFooter(m.results, w)
		return fmt.Sprintf("%s\n%s\n%s\n%s\n%s",
			header, sep, m.viewport.View(), sep, footer)
	}
	return ""
}

func renderDoctorFooter(results []doctor.Result, width int) string {
	var errs, warns, total int
	for _, r := range results {
		if r.Skipped {
			continue
		}
		for _, issue := range r.Issues {
			total++
			switch issue.Severity {
			case doctor.SeverityError:
				errs++
			case doctor.SeverityWarning:
				warns++
			}
		}
	}

	summary := fmt.Sprintf("%d issues · %d errors · %d warnings", total, errs, warns)
	keys := "↑↓ / pgup/pgdn scroll · q quit"

	gap := width - len([]rune(summary)) - len([]rune(keys))
	if gap < 2 {
		gap = 2
	}
	return styleDocCount.Render(summary) + strings.Repeat(" ", gap) + styleDocKeys.Render(keys)
}

const (
	colName = 20
	colDesc = 26
)

func renderDoctorResults(results []doctor.Result, specs []doctor.Spec, width int) string {
	byName := make(map[string]doctor.Result, len(results))
	for _, r := range results {
		byName[r.LinterName] = r
	}

	var b strings.Builder

	b.WriteString(styleDocSection.Render("SUMMARY") + "\n\n")
	for _, spec := range specs {
		r, ok := byName[spec.Linter.Name()]
		if !ok {
			continue
		}
		renderSummaryRow(&b, r)
	}

	hasIssues := false
	for _, spec := range specs {
		r := byName[spec.Linter.Name()]
		if r.Skipped || len(r.Issues) == 0 {
			continue
		}
		if !hasIssues {
			b.WriteString("\n")
			hasIssues = true
		}
		renderIssueGroup(&b, r, width)
	}

	if !hasIssues {
		b.WriteString("\n" + styleDocEnabled.Render("  ✓  No issues found — your library looks healthy!") + "\n")
	}

	for _, r := range results {
		if r.Err != nil {
			b.WriteString("\n" + styleDocError.Render(fmt.Sprintf("  ! %s failed: %v", r.LinterName, r.Err)) + "\n")
		}
	}

	return b.String()
}

func renderSummaryRow(b *strings.Builder, r doctor.Result) {
	if r.Skipped {
		icon := styleDocSkipped.Render("–")
		name := styleDocSkipped.Render(padRight(r.LinterName, colName))
		desc := styleDocSkipped.Render(padRight(r.LinterDesc, colDesc))
		reason := styleDocSkipped.Render(r.SkipReason)
		fmt.Fprintf(b, "  %s  %s  %s  %s\n", icon, name, desc, reason)
		return
	}
	icon := styleDocEnabled.Render("✓")
	if r.Err != nil {
		icon = styleDocError.Render("✗")
	}
	name := padRight(r.LinterName, colName)
	desc := padRight(r.LinterDesc, colDesc)
	var countStr string
	switch n := len(r.Issues); n {
	case 0:
		countStr = styleDocEnabled.Render("no issues")
	case 1:
		countStr = styleDocWarn.Render("1 issue")
	default:
		countStr = styleDocWarn.Render(fmt.Sprintf("%d issues", n))
	}
	dur := styleDocSkipped.Render(fmt.Sprintf("(%s)", r.Duration.Round(time.Millisecond)))
	fmt.Fprintf(b, "  %s  %s  %s  %-12s  %s\n", icon, name, desc, countStr, dur)
}

func renderIssueGroup(b *strings.Builder, r doctor.Result, width int) {
	heading := fmt.Sprintf("%s  %s",
		styleDocSection.Render(strings.ToUpper(r.LinterDesc)),
		styleDocCount.Render(fmt.Sprintf("%d issue(s)", len(r.Issues))),
	)
	b.WriteString(heading + "\n")
	sep := styleDocSep.Render(strings.Repeat("─", min(width, 72)))
	b.WriteString(sep + "\n")
	// "  ⚠  " prefix = 5 runes; leave a small right margin.
	maxPath := width - 7
	if maxPath < 40 {
		maxPath = 40
	}
	for _, issue := range r.Issues {
		var icon string
		switch issue.Severity {
		case doctor.SeverityError:
			icon = styleDocError.Render("✗")
		default:
			icon = styleDocWarn.Render("⚠")
		}
		path := styleDocIssue.Render(truncateLeft(issue.Path, maxPath))
		desc := styleDocSkipped.Render(issue.Description)
		fmt.Fprintf(b, "  %s  %s\n     %s\n", icon, path, desc)
	}
	b.WriteString("\n")
}

func buildSpecs(albums []beets.Album, items []beets.Item, libraryRoot string, cfg config.DoctorConfig) []doctor.Spec {
	thresholdBps := cfg.LowQualityThresholdKbps * 1000
	if thresholdBps == 0 {
		thresholdBps = 128_000
	}
	specs := []doctor.Spec{
		{Linter: linters.NewEmptyDirs(libraryRoot)},
		{Linter: linters.NewUntrackedDirs(libraryRoot, items)},
		{Linter: linters.NewLowQuality(items, thresholdBps)},
		{Linter: linters.NewLowercaseMetadata(items)},
		{Linter: linters.NewMissingArtwork(albums)},
		{Linter: linters.NewMissingYear(albums)},
	}
	for i := range specs {
		name := specs[i].Linter.Name()
		if enabled, ok := cfg.Linters[name]; ok && !enabled {
			specs[i].SkipReason = "disabled"
			continue
		}
		specs[i].Enabled = true
	}
	return specs
}

// selectSpecs keeps the specs named in names, in spec order. Empty names keeps all.
func selectSpecs(specs []doctor.Spec, names []string) ([]doctor.Spec, error) {
	if len(names) == 0 {
		return specs, nil
	}
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	var out []doctor.Spec
	for _, s := range specs {
		if want[s.Linter.Name()] {
			out = append(out, s)
			delete(want, s.Linter.Name())
		}
	}
	for n := range want {
		return nil, fmt.Errorf("unknown linter %q", n)
	}
	return out, nil
}

func runDoctor(_ *cobra.Command, _ []string) error {
	if err := requireFlag("db", flagDB); err != nil {
		return err
	}
	if err := requireFlag("beet", flagBeet); err != nil {
		return err
	}
	if flagDoctorPrint0 && !flagDoctorPaths {
		return fmt.Errorf("--print0 requires --paths")
	}
	if flagDoctorPaths && len(flagDoctorLinters) == 0 {
		return fmt.Errorf("--paths requires --linter (e.g. --linter empty_dirs)")
	}
	if _, err := selectSpecs(buildSpecs(nil, nil, "", loadedConfig.Doctor), flagDoctorLinters); err != nil {
		return err
	}
	library, err := beets.LibraryDir(flagBeet)
	if err != nil {
		return err
	}

	if flagDoctorPaths || flagDoctorJSON {
		return runDoctorHeadless(flagDB, library)
	}

	final, err := tea.NewProgram(newDoctorModel(flagDB, library, loadedConfig.Doctor)).Run()
	if err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return final.(doctorModel).err
}

type jsonReport struct {
	Library string       `json:"library"`
	Linters []jsonLinter `json:"linters"`
	Summary jsonSummary  `json:"summary"`
}

type jsonLinter struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Enabled     bool        `json:"enabled"`
	Skipped     bool        `json:"skipped,omitempty"`
	SkipReason  string      `json:"skip_reason,omitempty"`
	Error       string      `json:"error,omitempty"`
	DurationMs  int64       `json:"duration_ms"`
	Issues      []jsonIssue `json:"issues"`
}

type jsonIssue struct {
	Path        string `json:"path"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
}

type jsonSummary struct {
	Issues   int `json:"issues"`
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

// runDoctorHeadless loads the library, runs the (optionally filtered) linters
// synchronously, and writes machine-readable output instead of the TUI.
func runDoctorHeadless(dbPath, libraryRoot string) error {
	albums, err := beets.LoadAlbums(dbPath)
	if err != nil {
		return fmt.Errorf("loading albums: %w", err)
	}
	items, err := beets.LoadItems(dbPath)
	if err != nil {
		return fmt.Errorf("loading items: %w", err)
	}

	specs, err := selectSpecs(buildSpecs(albums, items, libraryRoot, loadedConfig.Doctor), flagDoctorLinters)
	if err != nil {
		return err
	}

	byName := make(map[string]doctor.Result)
	for r := range doctor.Run(context.Background(), specs) {
		byName[r.LinterName] = r
	}

	if flagDoctorPaths {
		return printDoctorPaths(specs, byName)
	}
	return printDoctorJSON(specs, byName, libraryRoot)
}

// printDoctorPaths writes one issue path per line (or NUL-separated with
// --print0). A skipped or failed linter is an error so callers see a non-zero
// exit instead of silently empty output.
func printDoctorPaths(specs []doctor.Spec, byName map[string]doctor.Result) error {
	sep := byte('\n')
	if flagDoctorPrint0 {
		sep = 0
	}
	w := bufio.NewWriter(os.Stdout)
	for _, s := range specs {
		r := byName[s.Linter.Name()]
		if r.Skipped {
			return fmt.Errorf("linter %q was skipped: %s", r.LinterName, r.SkipReason)
		}
		if r.Err != nil {
			return fmt.Errorf("linter %q failed: %w", r.LinterName, r.Err)
		}
		for _, iss := range r.Issues {
			w.WriteString(iss.Path)
			w.WriteByte(sep)
		}
	}
	return w.Flush()
}

// printDoctorJSON writes the full report in spec order so output is stable.
func printDoctorJSON(specs []doctor.Spec, byName map[string]doctor.Result, libraryRoot string) error {
	rep := jsonReport{Library: libraryRoot}
	for _, s := range specs {
		r := byName[s.Linter.Name()]
		jl := jsonLinter{
			Name:        s.Linter.Name(),
			Description: s.Linter.Description(),
			Enabled:     s.Enabled,
			Skipped:     r.Skipped,
			SkipReason:  r.SkipReason,
			DurationMs:  r.Duration.Milliseconds(),
		}
		if r.Err != nil {
			jl.Error = r.Err.Error()
		}
		for _, iss := range r.Issues {
			rep.Summary.Issues++
			switch iss.Severity {
			case doctor.SeverityError:
				rep.Summary.Errors++
			case doctor.SeverityWarning:
				rep.Summary.Warnings++
			}
			jl.Issues = append(jl.Issues, jsonIssue{
				Path:        iss.Path,
				Description: iss.Description,
				Severity:    string(iss.Severity),
			})
		}
		rep.Linters = append(rep.Linters, jl)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
