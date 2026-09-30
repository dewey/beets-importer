package linters

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
)

// SplitImports flags albums that beets split into several albums on import.
// Two cases count:
//
//   - albums in the same folder with the same name, like an album split by
//     featured artist. If two tracks share a title, the folder holds two
//     copies of the album instead, so it is left alone.
//   - one-track albums with the same album artist and name in different
//     folders, like a compilation imported track by track.
type SplitImports struct {
	albums []beets.Album
	items  []beets.Item
}

func NewSplitImports(albums []beets.Album, items []beets.Item) *SplitImports {
	return &SplitImports{albums: albums, items: items}
}

func (l *SplitImports) Name() string        { return "split_imports" }
func (l *SplitImports) Description() string { return "Split Imports" }

func (l *SplitImports) Run(ctx context.Context) ([]doctor.Issue, error) {
	var issues []doctor.Issue
	for _, g := range SplitImportGroups(l.albums, l.items) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		tracks := 0
		for _, a := range g {
			tracks += a.TrackCount
		}
		issues = append(issues, doctor.Issue{
			Path:        g[0].Path,
			AlbumID:     g[0].ID,
			Description: fmt.Sprintf("%s — %s: %d tracks split into %d albums", g[0].AlbumArtist, g[0].Album, tracks, len(g)),
			Severity:    doctor.SeverityWarning,
		})
	}
	return issues, nil
}

// SplitImportGroups returns the groups of albums that belong together, each
// sorted by album ID, and the groups sorted by name.
func SplitImportGroups(albums []beets.Album, items []beets.Item) [][]beets.Album {
	titles := make(map[int][]string)
	for _, it := range items {
		titles[it.AlbumID] = append(titles[it.AlbumID], strings.ToLower(strings.TrimSpace(it.Title)))
	}

	parent := make(map[int]int, len(albums))
	var find func(int) int
	find = func(id int) int {
		if parent[id] != id {
			parent[id] = find(parent[id])
		}
		return parent[id]
	}
	for _, a := range albums {
		parent[a.ID] = a.ID
	}
	union := func(g []beets.Album) {
		for _, a := range g[1:] {
			parent[find(a.ID)] = find(g[0].ID)
		}
	}

	byFolder := make(map[string][]beets.Album)
	byName := make(map[string][]beets.Album)
	for _, a := range albums {
		name := strings.ToLower(strings.TrimSpace(a.Album))
		if name == "" {
			continue
		}
		if a.Path != "" {
			byFolder[a.Path+"\x00"+name] = append(byFolder[a.Path+"\x00"+name], a)
		}
		if a.TrackCount == 1 {
			k := strings.ToLower(strings.TrimSpace(a.AlbumArtist)) + "\x00" + name
			byName[k] = append(byName[k], a)
		}
	}
	for _, g := range byFolder {
		if len(g) > 1 && !sharesTitle(g, titles) {
			union(g)
		}
	}
	for _, g := range byName {
		if len(g) > 1 {
			union(g)
		}
	}

	members := make(map[int][]beets.Album)
	for _, a := range albums {
		members[find(a.ID)] = append(members[find(a.ID)], a)
	}
	var groups [][]beets.Album
	for _, g := range members {
		if len(g) < 2 {
			continue
		}
		sort.Slice(g, func(i, j int) bool { return g[i].ID < g[j].ID })
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i][0].AlbumArtist+groups[i][0].Album < groups[j][0].AlbumArtist+groups[j][0].Album
	})
	return groups
}

func sharesTitle(g []beets.Album, titles map[int][]string) bool {
	seen := make(map[string]bool)
	for _, a := range g {
		for _, t := range titles[a.ID] {
			if seen[t] {
				return true
			}
			seen[t] = true
		}
	}
	return false
}
