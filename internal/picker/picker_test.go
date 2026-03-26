package picker

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dewey/beets-importer/internal/source"
)

func albums(names ...string) []source.Album {
	out := make([]source.Album, len(names))
	for i, n := range names {
		out[i] = source.Album{DirName: n, Path: "/music/" + n}
	}
	return out
}

func sendKey(m Model, key string) Model {
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return next.(Model)
}


func TestNew(t *testing.T) {
	m := New(albums("A", "B", "C"))
	if len(m.items) != 3 {
		t.Errorf("expected 3 items, got %d", len(m.items))
	}
	if m.Confirmed {
		t.Error("new model should not be confirmed")
	}
	for _, item := range m.items {
		if item.selected {
			t.Error("items should start unselected")
		}
	}
}

func TestSelected_empty(t *testing.T) {
	m := New(albums("A", "B"))
	if got := m.Selected(); len(got) != 0 {
		t.Errorf("expected 0 selected, got %d", len(got))
	}
}

func TestToggle_spaceSelects(t *testing.T) {
	m := New(albums("A", "B", "C"))
	// space toggles item at cursor (0)
	m = sendKey(m, " ")
	if !m.items[0].selected {
		t.Error("item 0 should be selected after space")
	}
	sel := m.Selected()
	if len(sel) != 1 || sel[0].Name != "A" {
		t.Errorf("unexpected selection: %v", sel)
	}

	// space again deselects
	m = sendKey(m, " ")
	if m.items[0].selected {
		t.Error("item 0 should be deselected after second space")
	}
}

func TestNavigation_downUp(t *testing.T) {
	m := New(albums("A", "B", "C"))
	if m.cursor != 0 {
		t.Fatalf("initial cursor should be 0, got %d", m.cursor)
	}
	m = sendKey(m, "j") // down
	if m.cursor != 1 {
		t.Errorf("cursor after j = %d, want 1", m.cursor)
	}
	m = sendKey(m, "k") // up
	if m.cursor != 0 {
		t.Errorf("cursor after k = %d, want 0", m.cursor)
	}
}

func TestNavigation_clampAtBounds(t *testing.T) {
	m := New(albums("A", "B"))
	// Can't go above 0
	m = sendKey(m, "k")
	if m.cursor != 0 {
		t.Errorf("cursor should stay 0 at top, got %d", m.cursor)
	}
	// Move to last item
	m = sendKey(m, "j")
	m = sendKey(m, "j") // already at last
	if m.cursor != 1 {
		t.Errorf("cursor should stop at last item (1), got %d", m.cursor)
	}
}

func TestSelectAll_togglesAll(t *testing.T) {
	m := New(albums("A", "B", "C"))

	// ctrl+a selects all when none selected
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	m = next.(Model)
	for i, item := range m.items {
		if !item.selected {
			t.Errorf("item %d should be selected after ctrl+a", i)
		}
	}

	// ctrl+a again deselects all
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	m = next.(Model)
	for i, item := range m.items {
		if item.selected {
			t.Errorf("item %d should be deselected after second ctrl+a", i)
		}
	}
}

func TestSelectAll_partialSelectsAll(t *testing.T) {
	m := New(albums("A", "B", "C"))
	// Select only item 0
	m = sendKey(m, " ")
	// ctrl+a should select all (not deselect, since not all are selected)
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	m = next.(Model)
	for i, item := range m.items {
		if !item.selected {
			t.Errorf("item %d should be selected after ctrl+a with partial selection", i)
		}
	}
}

func TestConfirm_enter(t *testing.T) {
	m := New(albums("A", "B"))
	if m.Confirmed {
		t.Fatal("should not be confirmed initially")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if !m.Confirmed {
		t.Error("should be confirmed after enter")
	}
	if cmd == nil {
		t.Error("enter should return a quit command")
	}
}

func TestQuit_doesNotConfirm(t *testing.T) {
	for _, key := range []string{"q", "esc"} {
		m := New(albums("A"))
		var keyType tea.KeyType
		switch key {
		case "q":
			keyType = tea.KeyRunes
		case "esc":
			keyType = tea.KeyEscape
		}
		var msg tea.KeyMsg
		if keyType == tea.KeyRunes {
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		} else {
			msg = tea.KeyMsg{Type: keyType}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
		if m.Confirmed {
			t.Errorf("key %q should not confirm", key)
		}
	}
}

func TestView_rendersWithoutPanic(t *testing.T) {
	m := New(albums("Artist — Album", "Another One"))
	_ = m.View() // just ensure no panic
}

func TestWindowResize_updatesHeight(t *testing.T) {
	m := New(albums("A"))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = next.(Model)
	// height = (40 - 5) / 2 = 17 (two lines per item)
	if m.height != 17 {
		t.Errorf("height = %d, want 17", m.height)
	}
}

func TestWindowResize_minHeight(t *testing.T) {
	m := New(albums("A"))
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 5})
	m = next.(Model)
	if m.height < 3 {
		t.Errorf("height should be at least 3, got %d", m.height)
	}
}

// ── NewFromItems ──────────────────────────────────────────────────────────────

func TestNewFromItems(t *testing.T) {
	items := []Item{
		{Name: "A", Path: "/music/a", Line1: "Artist A — Album A", Line2: "FLAC  2010"},
		{Name: "B", Path: "/music/b", Line1: "Artist B — Album B", Line2: "MP3  2005"},
	}
	m := NewFromItems(items)
	if len(m.items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(m.items))
	}
	if m.items[0].Path != "/music/a" {
		t.Errorf("item 0 path = %q, want /music/a", m.items[0].Path)
	}
	if m.items[1].Line2 != "MP3  2005" {
		t.Errorf("item 1 Line2 = %q, want MP3  2005", m.items[1].Line2)
	}
}

func TestNewFromItems_selectedReturnsSamePaths(t *testing.T) {
	items := []Item{
		{Name: "A", Path: "/music/a", Line1: "A"},
		{Name: "B", Path: "/music/b", Line1: "B"},
	}
	m := NewFromItems(items)
	m = sendKey(m, " ") // select item 0
	sel := m.Selected()
	if len(sel) != 1 || sel[0].Path != "/music/a" {
		t.Errorf("expected selected item with path /music/a, got %v", sel)
	}
}

// ── Empty list ────────────────────────────────────────────────────────────────

func TestNew_emptyAlbums(t *testing.T) {
	m := New(nil)
	if len(m.items) != 0 {
		t.Errorf("expected 0 items, got %d", len(m.items))
	}
}

func TestNew_emptyAlbums_navigationNoPanic(t *testing.T) {
	m := New(nil)
	// j/k/space on an empty list must not panic.
	for _, key := range []string{"j", "k", " "} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		m = next.(Model)
	}
	if m.cursor != 0 {
		t.Errorf("cursor should stay 0 on empty list, got %d", m.cursor)
	}
}

func TestNew_emptyAlbums_viewNoPanic(t *testing.T) {
	m := New(nil)
	_ = m.View()
}

// ── Scrolling offset ──────────────────────────────────────────────────────────

func TestScrolling_offsetAdvancesWhenCursorLeavesViewport(t *testing.T) {
	const numAlbums = 30
	als := make([]source.Album, numAlbums)
	for i := range als {
		als[i] = source.Album{DirName: fmt.Sprintf("Album %d", i), Path: fmt.Sprintf("/music/%d", i)}
	}
	m := New(als)
	// Set height so viewport shows 10 items.
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 15}) // height = 15-5 = 10
	m = next.(Model)

	// Move cursor past the visible area.
	for i := 0; i < 11; i++ {
		m = sendKey(m, "j")
	}
	if m.offset == 0 {
		t.Error("offset should have advanced when cursor moved past visible height")
	}
	if m.cursor > m.offset+m.height-1 {
		t.Errorf("cursor %d is out of visible range [%d, %d]", m.cursor, m.offset, m.offset+m.height-1)
	}
}

func TestScrolling_offsetRetractsWhenCursorReturnsToTop(t *testing.T) {
	const numAlbums = 30
	als := make([]source.Album, numAlbums)
	for i := range als {
		als[i] = source.Album{DirName: fmt.Sprintf("Album %d", i), Path: fmt.Sprintf("/music/%d", i)}
	}
	m := New(als)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 15})
	m = next.(Model)

	// Move down past the viewport, then back to the top.
	for i := 0; i < 15; i++ {
		m = sendKey(m, "j")
	}
	for i := 0; i < 15; i++ {
		m = sendKey(m, "k")
	}

	if m.cursor != 0 {
		t.Errorf("cursor = %d after returning to top, want 0", m.cursor)
	}
	if m.offset != 0 {
		t.Errorf("offset = %d after returning to top, want 0", m.offset)
	}
}

// ── Inspect modal ─────────────────────────────────────────────────────────────

func sendEsc(m Model) (Model, tea.Cmd) {
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	return next.(Model), cmd
}

func TestInspect_iOpensModal(t *testing.T) {
	m := New(albums("A", "B"))
	if m.inspect != nil {
		t.Fatal("inspect should be nil initially")
	}
	m = sendKey(m, "i")
	if m.inspect == nil {
		t.Fatal("inspect should be non-nil after pressing i")
	}
}

func TestInspect_escClosesModalWithoutQuitting(t *testing.T) {
	m := New(albums("A", "B"))
	m = sendKey(m, "i")
	if m.inspect == nil {
		t.Fatal("precondition: modal should be open")
	}
	m, cmd := sendEsc(m)
	if m.inspect != nil {
		t.Error("inspect should be nil after ESC closes modal")
	}
	if cmd != nil {
		t.Error("ESC closing modal should not return a quit command")
	}
}

func TestInspect_escFromListQuits(t *testing.T) {
	m := New(albums("A"))
	_, cmd := sendEsc(m)
	if cmd == nil {
		t.Error("ESC on the list (no modal open) should return a quit command")
	}
}

func TestInspect_keysIgnoredWhileModalOpen(t *testing.T) {
	m := New(albums("A", "B", "C"))
	m = sendKey(m, "i")
	cursorBefore := m.cursor

	// Navigation and toggle keys should be swallowed while the modal is open.
	for _, key := range []string{"j", "k", " "} {
		m = sendKey(m, key)
	}
	if m.cursor != cursorBefore {
		t.Errorf("cursor moved while modal was open: got %d, want %d", m.cursor, cursorBefore)
	}
	for _, item := range m.items {
		if item.selected {
			t.Error("item was selected while modal was open")
		}
	}
}

func TestInspect_sourcePath(t *testing.T) {
	items := []Item{
		{Name: "A", Path: "/music/source/A", LibraryPath: "/music/library/A", Line1: "A"},
	}
	m := NewFromItems(items)
	m = sendKey(m, "i")
	if m.inspect == nil {
		t.Fatal("modal should be open")
	}
	if m.inspect.sourcePath != "/music/source/A" {
		t.Errorf("sourcePath = %q, want /music/source/A", m.inspect.sourcePath)
	}
	if m.inspect.libraryPath != "/music/library/A" {
		t.Errorf("libraryPath = %q, want /music/library/A", m.inspect.libraryPath)
	}
}

func TestInspect_noLibraryPath(t *testing.T) {
	items := []Item{
		{Name: "A", Path: "/music/source/A", Line1: "A"},
	}
	m := NewFromItems(items)
	m = sendKey(m, "i")
	if m.inspect == nil {
		t.Fatal("modal should open even without a library path")
	}
	if m.inspect.libraryPath != "" {
		t.Errorf("libraryPath should be empty, got %q", m.inspect.libraryPath)
	}
	if m.inspect.libraryEntries != nil {
		t.Error("libraryEntries should be nil when no library path is set")
	}
}

func TestInspect_emptyListDoesNotOpen(t *testing.T) {
	m := New(nil)
	m = sendKey(m, "i")
	if m.inspect != nil {
		t.Error("inspect should not open on an empty list")
	}
}

func TestInspect_viewRendersWithoutPanic(t *testing.T) {
	items := []Item{
		{Name: "A", Path: "/music/source/A", LibraryPath: "/music/library/A", Line1: "A"},
	}
	m := NewFromItems(items)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	m = sendKey(m, "i")
	_ = m.View()
}

// ── readDirEntries ────────────────────────────────────────────────────────────

func TestReadDirEntries_realDirectory(t *testing.T) {
	dir := t.TempDir()
	// Create a mix of files and a subdirectory.
	files := []string{"01 - Track.flac", "02 - Track.flac", "cover.jpg"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	subdir := filepath.Join(dir, "extras")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	entries := readDirEntries(dir)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d: %v", len(entries), entries)
	}
	// os.ReadDir returns entries sorted by name; the subdir should have a trailing slash.
	foundDir := false
	for _, e := range entries {
		if e.name == "extras/" {
			foundDir = true
		}
	}
	if !foundDir {
		t.Errorf("expected 'extras/' in entries, got %v", entries)
	}
}

func TestReadDirEntries_emptyDirectory(t *testing.T) {
	dir := t.TempDir()
	entries := readDirEntries(dir)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty dir, got %d", len(entries))
	}
}

func TestReadDirEntries_nonExistentPath(t *testing.T) {
	entries := readDirEntries("/this/path/does/not/exist")
	if len(entries) != 1 {
		t.Fatalf("expected 1 error entry, got %d", len(entries))
	}
	if entries[0].name[:7] != "(error:" {
		t.Errorf("expected error entry, got %q", entries[0].name)
	}
}

func TestReadDirEntries_emptyPathReturnsNil(t *testing.T) {
	entries := readDirEntries("")
	if entries != nil {
		t.Errorf("expected nil for empty path, got %v", entries)
	}
}

// ── truncatePath ──────────────────────────────────────────────────────────────

func TestTruncatePath(t *testing.T) {
	cases := []struct {
		input string
		max   int
		want  string
	}{
		{"/short", 20, "/short"},
		{"/a/b/c/d/e/f/verylongpath", 10, "…ylongpath"}, // last 9 chars of the input
		{"exactly10c", 10, "exactly10c"},
		{"/a/b/c/d/e", 4, "…d/e"}, // last 3 chars of the input
		{"abc", 2, "ab"},
	}
	for _, tc := range cases {
		got := truncatePath(tc.input, tc.max)
		if got != tc.want {
			t.Errorf("truncatePath(%q, %d) = %q, want %q", tc.input, tc.max, got, tc.want)
		}
	}
}

// ── renderDirListing ──────────────────────────────────────────────────────────

func TestRenderDirListing_truncatesLongLists(t *testing.T) {
	entries := make([]dirEntry, 20)
	for i := range entries {
		entries[i] = dirEntry{name: fmt.Sprintf("track%02d.flac", i+1), size: "10.0 MB"}
	}
	out := renderDirListing(entries, 5, 40)
	// Should mention that more entries were omitted.
	if out == "" {
		t.Fatal("expected non-empty output")
	}
	// The "and N more" line must appear since 20 > 5.
	if !containsSubstring(out, "more") {
		t.Errorf("expected truncation notice in output, got:\n%s", out)
	}
}

func TestRenderDirListing_emptyEntries(t *testing.T) {
	out := renderDirListing(nil, 10, 40)
	if out == "" {
		t.Error("expected non-empty output for empty entries")
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
