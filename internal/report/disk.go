package report

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Disk reads facts from the music folder that beets does not store.
type Disk interface {
	// Size returns the size of the file, or 0 when it is missing.
	Size(path string) (int64, error)
	// CoverFile returns the file name of a cover image in the folder, or "".
	CoverFile(dir string) (string, error)
}

var coverNames = map[string]bool{
	"cover": true, "folder": true, "front": true, "album": true, "albumart": true, "art": true, "artwork": true,
}

var imageExts = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true}

// DiskCache answers from known facts and reads the disk for the rest. It
// remembers what it read, so the caller can store it.
type DiskCache struct {
	knownSizes  map[string]int64
	knownCovers map[string]string
	sizes       map[string]int64
	covers      map[string]string
}

func NewDiskCache(sizes map[string]int64, covers map[string]string) *DiskCache {
	return &DiskCache{knownSizes: sizes, knownCovers: covers, sizes: map[string]int64{}, covers: map[string]string{}}
}

// Read returns the facts read from the disk, not the known ones.
func (c *DiskCache) Read() (sizes map[string]int64, covers map[string]string) {
	return c.sizes, c.covers
}

func (c *DiskCache) Size(path string) (int64, error) {
	if n, ok := c.knownSizes[path]; ok {
		return n, nil
	}
	fi, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		c.sizes[path] = 0
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", path, err)
	}
	c.sizes[path] = fi.Size()
	return fi.Size(), nil
}

func (c *DiskCache) CoverFile(dir string) (string, error) {
	if name, ok := c.knownCovers[dir]; ok {
		return name, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read folder %s: %w", dir, err)
	}
	name := ""
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		base := strings.ToLower(strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())))
		if !e.IsDir() && imageExts[ext] && coverNames[base] {
			name = e.Name()
			break
		}
	}
	c.covers[dir] = name
	return name, nil
}
