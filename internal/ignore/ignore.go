package ignore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/text/unicode/norm"
)

// Feature names keep the ignores of one command apart from the others.
const Upgrades = "upgrades"

// Entry is one ignored item. Library is empty for features that only ignore a source.
type Entry struct {
	Source  string `json:"source"`
	Library string `json:"library,omitempty"`
}

// Store is a persistent list of ignored entries, grouped by feature. It is
// user data, so it lives next to the config file and not in the cache dir.
type Store struct {
	path    string
	entries map[string]map[Entry]bool
	dirty   bool
}

// Load reads the store from path. A missing file gives an empty store.
func Load(path string) (*Store, error) {
	s := &Store{path: path, entries: map[string]map[Entry]bool{}}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read ignore file %s: %w", path, err)
	}
	var byFeature map[string][]Entry
	if err = json.Unmarshal(data, &byFeature); err != nil {
		return nil, fmt.Errorf("parse ignore file %s: %w", path, err)
	}
	for feature, entries := range byFeature {
		for _, e := range entries {
			s.set(feature, e, true)
		}
	}
	s.dirty = false
	return s, nil
}

// Has reports whether the entry is ignored for the feature.
func (s *Store) Has(feature string, e Entry) bool {
	return s.entries[feature][normalize(e)]
}

// Set ignores or unignores the entry. Call Save to persist it.
func (s *Store) Set(feature string, e Entry, ignored bool) {
	s.set(feature, e, ignored)
}

func (s *Store) set(feature string, e Entry, ignored bool) {
	e = normalize(e)
	if s.entries[feature][e] == ignored {
		return
	}
	s.dirty = true
	if !ignored {
		delete(s.entries[feature], e)
		return
	}
	if s.entries[feature] == nil {
		s.entries[feature] = map[Entry]bool{}
	}
	s.entries[feature][e] = true
}

// Save writes the store to disk if it changed, creating the parent directory if needed.
func (s *Store) Save() error {
	if !s.dirty {
		return nil
	}
	byFeature := map[string][]Entry{}
	for feature, entries := range s.entries {
		for e := range entries {
			byFeature[feature] = append(byFeature[feature], e)
		}
	}
	data, err := json.MarshalIndent(byFeature, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ignore file: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create ignore dir: %w", err)
	}
	if err = os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write ignore file %s: %w", s.path, err)
	}
	s.dirty = false
	return nil
}

// normalize uses NFC because the SMB share can return either form of a name.
func normalize(e Entry) Entry {
	return Entry{norm.NFC.String(e.Source), norm.NFC.String(e.Library)}
}
