package picker

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dewey/beets-importer/internal/source"
)

var (
	styleCursor   = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	styleCheck    = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	styleSelected = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleNormal   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleMeta     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleSep      = lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
	styleStatus   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleHeader   = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// Item is one entry in the picker list.
type Item struct {
	Name        string // short display name used in import progress output
	Path        string // filesystem path passed to beet import
	LibraryPath string // directory of the matched library album (for inspect; may be empty)
	Line1       string // main display line shown in the picker
	Line2       string // second display line shown below Line1 (optional)
	Styled      bool   // if true, Line1/Line2 already contain ANSI codes; skip style wrapping
	selected    bool
}

// dirEntry holds a single file/directory name and its human-readable size.
type dirEntry struct {
	name string
	size string // empty for directories
}

// inspectState holds the directory listings shown in the inspect modal.
type inspectState struct {
	sourcePath     string
	libraryPath    string
	sourceEntries  []dirEntry
	libraryEntries []dirEntry
}

// Model is the bubbletea model for the multi-select album picker.
type Model struct {
	items        []Item
	cursor       int
	offset       int
	height       int // visible rows for the list, updated on WindowSizeMsg
	width        int // terminal width, updated on WindowSizeMsg
	windowHeight int // full terminal height, updated on WindowSizeMsg
	Confirmed    bool
	inspect      *inspectState // non-nil when the inspect modal is open
}

// New creates a Model from a slice of source albums.
func New(albums []source.Album) Model {
	items := make([]Item, len(albums))
	for i, a := range albums {
		line1 := a.DirName
		if a.Artist != "" && a.Album != "" {
			line1 = a.Artist + " — " + a.Album
		}
		var meta []string
		if a.Format != "" {
			meta = append(meta, a.Format)
		}
		if a.Year > 0 {
			meta = append(meta, strconv.Itoa(a.Year))
		}
		items[i] = Item{
			Name:  a.DirName,
			Path:  a.Path,
			Line1: line1,
			Line2: strings.Join(meta, "  "),
		}
	}
	return Model{items: items, height: 20, width: 90}
}

// NewFromItems creates a Model from a pre-built slice of Items.
// Use this when the caller needs full control over display text (e.g. upgrade candidates).
func NewFromItems(items []Item) Model {
	return Model{items: items, height: 20, width: 90}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve rows for header (2), separator (1), footer sep (1), status (1).
		// Each item takes 2 lines (main + meta).
		m.width = msg.Width
		m.windowHeight = msg.Height
		m.height = (msg.Height - 5) / 2
		if m.height < 3 {
			m.height = 3
		}

	case tea.KeyPressMsg:
		// When inspect modal is open, only ESC is handled (to close it).
		if m.inspect != nil {
			if msg.String() == "esc" {
				m.inspect = nil
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit

		case "enter":
			m.Confirmed = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.offset {
					m.offset = m.cursor
				}
			}

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				if m.cursor >= m.offset+m.height {
					m.offset = m.cursor - m.height + 1
				}
			}

		case "space":
			if len(m.items) > 0 {
				m.items[m.cursor].selected = !m.items[m.cursor].selected
			}

		case "ctrl+a":
			// If any item is unselected, select all. Otherwise deselect all.
			allSelected := true
			for _, item := range m.items {
				if !item.selected {
					allSelected = false
					break
				}
			}
			for i := range m.items {
				m.items[i].selected = !allSelected
			}

		case "i":
			if len(m.items) > 0 {
				m.inspect = buildInspectState(m.items[m.cursor])
			}
		}
	}
	return m, nil
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	base := m.viewList()
	if m.inspect == nil {
		return base
	}

	modal := m.viewInspect()
	modalW := lipgloss.Width(modal)
	modalH := lipgloss.Height(modal)
	x := (m.width - modalW) / 2
	y := (m.windowHeight - modalH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	bg := lipgloss.NewLayer(base)
	overlay := lipgloss.NewLayer(modal).X(x).Y(y).Z(1)
	return lipgloss.NewCompositor(bg, overlay).Render()
}

func (m Model) viewList() string {
	var b strings.Builder

	b.WriteString(styleHeader.Render("Select albums to import"))
	b.WriteString("  ")
	b.WriteString(styleMeta.Render("SPACE toggle · CTRL+A all · I inspect · ENTER confirm · ESC cancel"))
	b.WriteString("\n")
	b.WriteString(styleSep.Render(strings.Repeat("─", 90)))
	b.WriteString("\n")

	end := m.offset + m.height
	if end > len(m.items) {
		end = len(m.items)
	}
	for i, item := range m.items[m.offset:end] {
		idx := m.offset + i

		cursor := "  "
		if idx == m.cursor {
			cursor = styleCursor.Render("▶ ")
		}

		check := styleNormal.Render("[ ]")
		nameStyle := styleNormal
		if item.selected {
			check = styleCheck.Render("[✓]")
			nameStyle = styleSelected
		}

		line1 := item.Line1
		if !item.Styled {
			line1 = nameStyle.Render(line1)
		}
		fmt.Fprintf(&b, "%s%s %s\n", cursor, check, line1)
		if item.Line2 != "" {
			line2 := item.Line2
			if !item.Styled {
				line2 = styleMeta.Render(line2)
			}
			fmt.Fprintf(&b, "      %s\n", line2)
		}
	}

	b.WriteString(styleSep.Render(strings.Repeat("─", 90)))
	b.WriteString("\n")

	nSelected := m.numSelected()
	scroll := ""
	if len(m.items) > m.height {
		scroll = fmt.Sprintf("  (%d–%d of %d)", m.offset+1, end, len(m.items))
	}
	b.WriteString(styleStatus.Render(
		fmt.Sprintf("%d selected%s", nSelected, scroll),
	))

	return b.String()
}

func (m Model) viewInspect() string {
	s := m.inspect

	// Modal uses up to 90% of terminal width, split across two panels.
	modalWidth := m.width * 9 / 10
	if modalWidth < 44 {
		modalWidth = 44
	}
	panelInner := modalWidth/2 - 4 // subtract border (2) + padding (2) per panel
	if panelInner < 18 {
		panelInner = 18
	}

	// Lines available inside each panel: modal height capped at 80% of terminal.
	listHeight := m.windowHeight * 4 / 5
	if listHeight < 3 {
		listHeight = 3
	}

	srcTitle := styleMeta.Render("Source: ") + styleNormal.Render(truncatePath(s.sourcePath, panelInner-9))
	libTitle := styleMeta.Render("Library: ") + styleNormal.Render(truncatePath(s.libraryPath, panelInner-10))
	if s.libraryPath == "" {
		libTitle = styleMeta.Render("Library: ") + styleDim.Render("(not available)")
	}

	srcListing := renderDirListing(s.sourceEntries, listHeight, panelInner)
	libListing := renderDirListing(s.libraryEntries, listHeight, panelInner)

	panelStyle := func(borderColor string) lipgloss.Style {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(borderColor)).
			Padding(0, 1).
			Width(panelInner)
	}

	srcPanel := panelStyle("51").Render(
		srcTitle + "\n" + styleSep.Render(strings.Repeat("─", panelInner)) + "\n" + srcListing,
	)
	libPanel := panelStyle("82").Render(
		libTitle + "\n" + styleSep.Render(strings.Repeat("─", panelInner)) + "\n" + libListing,
	)

	panels := lipgloss.JoinHorizontal(lipgloss.Top, srcPanel, libPanel)
	footer := "\n" + styleMeta.Render("ESC  close inspector")

	return panels + footer
}

// buildInspectState reads directory listings for source and library paths.
func buildInspectState(item Item) *inspectState {
	s := &inspectState{
		sourcePath:  item.Path,
		libraryPath: item.LibraryPath,
	}
	s.sourceEntries = readDirEntries(item.Path)
	if item.LibraryPath != "" {
		s.libraryEntries = readDirEntries(item.LibraryPath)
	}
	return s
}

func readDirEntries(path string) []dirEntry {
	if path == "" {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return []dirEntry{{name: "(error: " + err.Error() + ")"}}
	}
	out := make([]dirEntry, len(entries))
	for i, e := range entries {
		name := e.Name()
		var size string
		if e.IsDir() {
			name += "/"
		} else if info, err := e.Info(); err == nil {
			size = humanSize(info.Size())
		}
		out[i] = dirEntry{name: name, size: size}
	}
	return out
}

func renderDirListing(entries []dirEntry, maxLines, width int) string {
	if len(entries) == 0 {
		return styleDim.Render("(empty)")
	}
	shown := entries
	more := 0
	if len(entries) > maxLines {
		shown = entries[:maxLines]
		more = len(entries) - maxLines
	}
	var b strings.Builder
	for _, e := range shown {
		if e.size == "" {
			// directory: full width, no size column
			b.WriteString(styleDim.Render(e.name))
		} else {
			// file: name left-aligned, size right-aligned
			nameWidth := width - len(e.size)
			if nameWidth < 1 {
				nameWidth = 1
			}
			name := e.name
			if len(name) > nameWidth {
				name = name[:nameWidth-1] + "…"
			}
			pad := nameWidth - len([]rune(name))
			if pad < 0 {
				pad = 0
			}
			b.WriteString(styleNormal.Render(name))
			b.WriteString(strings.Repeat(" ", pad))
			b.WriteString(styleMeta.Render(e.size))
		}
		b.WriteByte('\n')
	}
	if more > 0 {
		b.WriteString(styleDim.Render(fmt.Sprintf("… and %d more", more)))
	}
	return b.String()
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func truncatePath(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max < 4 {
		return s[:max]
	}
	return "…" + s[len(s)-(max-1):]
}

// Selected returns the items the user confirmed.
func (m Model) Selected() []Item {
	out := make([]Item, 0, len(m.items))
	for _, item := range m.items {
		if item.selected {
			out = append(out, item)
		}
	}
	return out
}

func (m Model) numSelected() int {
	n := 0
	for _, item := range m.items {
		if item.selected {
			n++
		}
	}
	return n
}
