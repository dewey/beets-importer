package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ScanCache is a persistent, mtime-keyed cache of directory scan results.
// Each entry is keyed by the directory path, its modification time, and
// whether bitrate probing was enabled. An entry is automatically a miss
// when the directory's mtime changes (e.g. files were added or removed).
//
// The cache file can be safely deleted at any time — the next run will
// do a full scan and rebuild it from scratch.
type ScanCache struct {
	path    string
	dirty   bool
	entries map[string]Album
}

// DefaultCachePath returns the default location for the scan cache file.
// On macOS this is ~/Library/Caches/beets-importer/scan-cache.json.
// On Linux it is ~/.cache/beets-importer/scan-cache.json.
func DefaultCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("user cache dir: %w", err)
	}
	return filepath.Join(dir, "beets-importer", "scan-cache.json"), nil
}

// LoadCache loads the cache from path. If the file does not exist an empty
// cache is returned. If the file is corrupt it is silently discarded and an
// empty cache is returned.
func LoadCache(path string) (*ScanCache, error) {
	c := &ScanCache{path: path, entries: make(map[string]Album)}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return nil, fmt.Errorf("read scan cache %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &c.entries); err != nil {
		// Corrupt cache — start fresh rather than failing.
		c.entries = make(map[string]Album)
	}
	return c, nil
}

// Save writes the cache to disk. It is a no-op if nothing has changed since
// the last load or save. The parent directory is created if it does not exist.
func (c *ScanCache) Save() error {
	if !c.dirty {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	data, err := json.Marshal(c.entries)
	if err != nil {
		return fmt.Errorf("marshal scan cache: %w", err)
	}
	if err := os.WriteFile(c.path, data, 0o644); err != nil {
		return fmt.Errorf("write scan cache: %w", err)
	}
	c.dirty = false
	return nil
}

func cacheKey(dirPath string, mtime time.Time, withBitrate bool) string {
	b := byte('0')
	if withBitrate {
		b = '1'
	}
	return fmt.Sprintf("%s\x00%d\x00%c", dirPath, mtime.Unix(), b)
}

func (c *ScanCache) lookup(dirPath string, mtime time.Time, withBitrate bool) (Album, bool) {
	a, ok := c.entries[cacheKey(dirPath, mtime, withBitrate)]
	return a, ok
}

func (c *ScanCache) store(dirPath string, mtime time.Time, withBitrate bool, a Album) {
	c.entries[cacheKey(dirPath, mtime, withBitrate)] = a
	c.dirty = true
}
