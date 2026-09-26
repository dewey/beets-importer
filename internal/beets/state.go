package beets

import (
	"fmt"
	"path/filepath"

	"github.com/nlpodyssey/gopickle/pickle"
	"github.com/nlpodyssey/gopickle/types"
	"golang.org/x/text/unicode/norm"
)

// ProcessedPaths reads the folders beets has already handled (applied or
// skipped) from its incremental state file, the same list 'beet import -i'
// uses to skip folders.
//
// beets stores the leaf folders that hold the music, so a multi-disc album is
// stored as "X/CD1" and "X/CD2". Every parent folder is added too, so a lookup
// for "X" finds it.
//
// Paths are NFC normalized. The SMB share can return either form for the same
// name, so both sides must be normalized before comparing.
func ProcessedPaths(stateFile string) (map[string]bool, error) {
	v, err := pickle.Load(stateFile)
	if err != nil {
		return nil, fmt.Errorf("read beets state %s: %w", stateFile, err)
	}
	state, ok := v.(*types.Dict)
	if !ok {
		return nil, fmt.Errorf("beets state %s: expected dict, got %T", stateFile, v)
	}
	h, ok := state.Get("taghistory")
	if !ok {
		return nil, fmt.Errorf("beets state %s: no taghistory, is incremental enabled in beets?", stateFile)
	}
	history, ok := h.(*types.Set)
	if !ok {
		return nil, fmt.Errorf("beets state %s: taghistory: expected set, got %T", stateFile, h)
	}
	paths := make(map[string]bool, history.Len())
	for entry := range *history {
		// Each entry is a tuple of the album's directory paths as bytes.
		t, ok := entry.(*types.Tuple)
		if !ok {
			return nil, fmt.Errorf("beets state %s: taghistory entry: expected tuple, got %T", stateFile, entry)
		}
		for i := range t.Len() {
			b, ok := t.Get(i).([]byte)
			if !ok {
				return nil, fmt.Errorf("beets state %s: taghistory path: expected bytes, got %T", stateFile, t.Get(i))
			}
			for p := norm.NFC.String(string(b)); p != filepath.Dir(p); p = filepath.Dir(p) {
				paths[p] = true
			}
		}
	}
	return paths, nil
}
