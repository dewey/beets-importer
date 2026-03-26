package source

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dhowden/tag"
	"golang.org/x/text/unicode/norm"
)

// Album represents an album directory found in the source (download) directory.
type Album struct {
	DirName    string
	Path       string
	Artist     string
	Album      string
	Year       int
	Format     string // dominant audio format: "FLAC", "MP3", etc.
	AvgBitrate int    // average bitrate in bps (via ffprobe), 0 if unknown
	TrackCount int
	HasArtwork bool
}

var (
	audioExts = map[string]string{
		".flac": "FLAC",
		".mp3":  "MP3",
		".m4a":  "AAC",
		".aac":  "AAC",
		".ogg":  "OGG",
		".opus": "OPUS",
		".wav":  "WAV",
		".aiff": "AIFF",
		".ape":  "APE",
	}

	artworkNames = []string{
		"cover.jpg", "cover.jpeg", "cover.png",
		"folder.jpg", "folder.jpeg", "folder.png",
		"front.jpg", "front.jpeg", "front.png",
		"artwork.jpg", "artwork.png",
	}

	// Matches a year: 4-digit number 1900-2099
	yearRe = regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`)

	// Strip content inside brackets/parens that look like format/quality tags
	// e.g. [FLAC], [320], [V0], [WEB], [MP3 320kbps], (FLAC), etc.
	bracketTagRe = regexp.MustCompile(`[\[\(][^\]\)]{1,30}[\]\)]`)

	// Strip leading bracket group like "[Label]" at start of name
	leadingBracketRe = regexp.MustCompile(`^\[[^\]]+\]\s*`)
)

// ScanOptions controls optional behaviour during scanning.
type ScanOptions struct {
	// Bitrate runs ffprobe on each directory to determine average bitrate.
	// Slower but required for same-format upgrade comparisons.
	Bitrate bool
	// OnWarn is called instead of printing to stderr when a directory is
	// skipped due to an error. If nil, warnings are printed to stderr.
	OnWarn func(dirName string, err error)
	// Cache, if non-nil, is used to skip re-scanning directories whose
	// modification time has not changed since the last run.
	Cache *ScanCache
}

// ScanEach walks the top-level directories under root and calls fn for each
// directory that contains audio files. If fn returns false, scanning stops early.
// This allows the caller to apply a limit or update a progress indicator between dirs.
// A cancelled ctx stops the scan between directories, even non-audio ones.
func ScanEach(ctx context.Context, root string, opts ScanOptions, fn func(a Album, done, total int) (keepGoing bool)) error {
	if opts.Bitrate {
		if _, err := exec.LookPath("ffprobe"); err != nil {
			return fmt.Errorf("ffprobe not found in PATH: install ffmpeg (https://ffmpeg.org/download.html) to enable bitrate scanning")
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read source dir %s: %w", root, err)
	}

	// Count only directories upfront so callers can show progress.
	total := 0
	for _, e := range entries {
		if e.IsDir() {
			total++
		}
	}

	if opts.Cache != nil {
		defer opts.Cache.Save() //nolint:errcheck
	}

	done := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if ctx.Err() != nil {
			return nil
		}
		done++
		name := entry.Name()

		// Attempt a cache lookup before doing any I/O on the directory.
		if opts.Cache != nil {
			if info, err := entry.Info(); err == nil {
				cacheDir := filepath.Join(root, name)
				if album, ok := opts.Cache.lookup(cacheDir, info.ModTime(), opts.Bitrate); ok {
					if !fn(album, done, total) {
						return nil
					}
					continue
				}
			}
		}

		_, album, ok, err := scanDirWithFallback(root, name, opts.Bitrate, opts.OnWarn)
		if err != nil {
			if opts.OnWarn != nil {
				opts.OnWarn(name, err)
			} else {
				fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", name, err)
			}
			continue
		}
		if ok {
			if opts.Cache != nil {
				if info, err2 := entry.Info(); err2 == nil {
					opts.Cache.store(filepath.Join(root, name), info.ModTime(), opts.Bitrate, album)
				}
			}
			if !fn(album, done, total) {
				return nil
			}
		}
	}
	return nil
}

// QuickScanDir scans a single directory without running ffprobe.
// Used by the latest subcommand where bitrate is not needed.
// If cache is non-nil and mtime is the directory's modification time, the
// result may be served from cache and the on-disk scan skipped entirely.
// Pass nil/zero to opt out of caching.
func QuickScanDir(dirPath, dirName string, cache *ScanCache, mtime time.Time) (Album, bool, error) {
	if cache != nil && !mtime.IsZero() {
		if album, ok := cache.lookup(dirPath, mtime, false); ok {
			return album, true, nil
		}
	}
	album, ok, err := scanDir(dirPath, dirName, false, nil)
	if err == nil && ok && cache != nil && !mtime.IsZero() {
		cache.store(dirPath, mtime, false, album)
	}
	return album, ok, err
}

// scanDirWithFallback constructs the directory path from root+name and calls
// scanDir. If the OS returns "not found" — which can happen on macOS when the
// VFS layer normalizes Unicode path components differently from the form the
// network filesystem (NFS/SMB) used to store them — it retries with NFC and
// NFD normalizations of the name before giving up.
// Returns the path variant that actually succeeded alongside the scan result.
func scanDirWithFallback(root, name string, withBitrate bool, onWarn func(string, error)) (string, Album, bool, error) {
	candidates := []string{
		name,
		norm.NFC.String(name),
		norm.NFD.String(name),
	}
	// Deduplicate while preserving order so we don't retry identical paths.
	seen := make(map[string]bool)
	var last error
	for _, n := range candidates {
		if seen[n] {
			continue
		}
		seen[n] = true
		p := filepath.Join(root, n)
		album, ok, err := scanDir(p, name, withBitrate, onWarn)
		if err == nil {
			return p, album, ok, nil
		}
		if !os.IsNotExist(err) {
			// Non-existence errors (permissions, I/O) are returned immediately.
			return p, Album{}, false, err
		}
		last = err
	}
	return filepath.Join(root, name), Album{}, false, last
}

func scanDir(dirPath, dirName string, withBitrate bool, onWarn func(string, error)) (Album, bool, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return Album{}, false, err
	}

	// Collect audio files and check artwork
	formatCounts := make(map[string]int)
	var audioFiles []string
	hasArtwork := false

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if audioFmt, ok := audioExts[filepath.Ext(name)]; ok {
			formatCounts[audioFmt]++
			audioFiles = append(audioFiles, filepath.Join(dirPath, e.Name()))
		}
		for _, art := range artworkNames {
			if name == art {
				hasArtwork = true
			}
		}
	}

	// Also recurse one level for multi-disc layouts (disc 1/, disc 2/, CD1/, etc.)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(dirPath, e.Name())
		subEntries, err := os.ReadDir(sub)
		if err != nil {
			if onWarn != nil {
				onWarn(sub, err)
			}
			continue
		}
		for _, se := range subEntries {
			if se.IsDir() {
				continue
			}
			name := strings.ToLower(se.Name())
			if audioFmt, ok := audioExts[filepath.Ext(name)]; ok {
				formatCounts[audioFmt]++
				audioFiles = append(audioFiles, filepath.Join(sub, se.Name()))
			}
			for _, art := range artworkNames {
				if name == art {
					hasArtwork = true
				}
			}
		}
	}

	if len(audioFiles) == 0 {
		return Album{}, false, nil
	}

	// Dominant format
	dominantFmt := ""
	maxCount := 0
	for f, c := range formatCounts {
		if c > maxCount {
			maxCount = c
			dominantFmt = f
		}
	}

	// Sample up to 3 files for bitrate via ffprobe (only when requested)
	avgBitrate := 0
	if withBitrate {
		avgBitrate = probeAvgBitrate(audioFiles, 3)
	}

	// Read artist/album/year from tags of the first file; fall back to directory name parsing.
	artist, album, year := readTagsFromFile(audioFiles[0])
	if artist == "" && album == "" {
		artist, album, year = parseDirName(dirName)
	} else {
		if year == 0 {
			// First file had no year tag; try up to 4 more files before giving up on tags.
			for _, f := range audioFiles[1:min(5, len(audioFiles))] {
				_, _, y := readTagsFromFile(f)
				if y > 0 {
					year = y
					break
				}
			}
		}
		if year == 0 {
			// Still no year from tags — fall back to directory name.
			_, _, dirYear := parseDirName(dirName)
			if dirYear != 0 {
				year = dirYear
			}
		}
	}

	return Album{
		DirName:    dirName,
		Path:       dirPath,
		Artist:     artist,
		Album:      album,
		Year:       year,
		Format:     dominantFmt,
		AvgBitrate: avgBitrate,
		TrackCount: len(audioFiles),
		HasArtwork: hasArtwork,
	}, true, nil
}

// probeAvgBitrate runs ffprobe on up to maxSample audio files and returns
// the average bit_rate in bps. Returns 0 if ffprobe is unavailable or fails.
func probeAvgBitrate(files []string, maxSample int) int {
	if len(files) > maxSample {
		files = files[:maxSample]
	}
	var total, count int
	for _, f := range files {
		out, err := exec.Command(
			"ffprobe",
			"-v", "error",
			"-show_entries", "format=bit_rate",
			"-of", "default=noprint_wrappers=1:nokey=1",
			f,
		).Output()
		if err != nil {
			continue
		}
		val := strings.TrimSpace(string(out))
		if val == "N/A" || val == "" {
			continue
		}
		bps, err := strconv.Atoi(val)
		if err != nil {
			continue
		}
		total += bps
		count++
	}
	if count == 0 {
		return 0
	}
	return total / count
}

// readTagsFromFile reads artist, album, and year from the audio file's embedded tags.
// Returns empty strings/zero if the file can't be read or tags are absent.
func readTagsFromFile(path string) (artist, album string, year int) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return
	}

	// Prefer AlbumArtist over track Artist for album-level matching
	artist = strings.TrimSpace(m.AlbumArtist())
	if artist == "" {
		artist = strings.TrimSpace(m.Artist())
	}
	album = strings.TrimSpace(m.Album())
	year = m.Year()
	return
}

// parseDirName extracts artist, album, and year from a directory name.
// Handles common torrent naming conventions.
func parseDirName(name string) (artist, album string, year int) {
	// Strip leading bracket group e.g. "[Warp Records] Boards of Canada - ..."
	name = leadingBracketRe.ReplaceAllString(name, "")

	// Extract year before we strip brackets
	if m := yearRe.FindString(name); m != "" {
		year, _ = strconv.Atoi(m)
	}

	// Strip all bracket/paren tags: [FLAC], [320kbps], (2003), [WEB], etc.
	name = bracketTagRe.ReplaceAllString(name, "")
	name = strings.TrimSpace(name)

	// Split on " - " to get artist and album
	parts := strings.SplitN(name, " - ", 2)
	if len(parts) == 2 {
		artist = strings.TrimSpace(parts[0])
		album = strings.TrimSpace(parts[1])
	} else {
		// No " - " separator: treat whole name as album
		album = strings.TrimSpace(name)
	}

	// Clean trailing/leading punctuation artifacts
	artist = strings.Trim(artist, " -_.")
	album = strings.Trim(album, " -_.")
	return
}
