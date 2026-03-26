# beets-importer

A CLI tool for [beets](https://beets.readthedocs.io) that helps with two things:

- **`import`**: pick recently-added albums from your download folder and import them interactively, or import from a text file of paths
- **`upgrades`**: scan your download folder, match albums against your library, and surface upgrade candidates: better format (e.g. FLAC replacing MP3) or higher bitrate

## Screenshots

The core problem: beets' own importer processes albums one-by-one in sequence. If you have a large inbox, you can't skim everything at once, pick only what you want right now, and skip the rest — you're stuck in a linear flow.

`beets-importer` adds a TUI in front of `beet import` so you can batch-select albums before anything gets imported.

**Upgrade picker** — scans your inbox, matches albums against your beets library, and shows upgrade candidates (e.g. FLAC replacing MP3) side by side with the library copy. Select any subset, then import them all in one go:

![Upgrades picker](docs/screenshot-picker.png)

**Inspector** — press `I` on any candidate to open a side-by-side file listing of the source and library directories, so you can verify the match before committing:

![Inspector](docs/screenshot-inspector.png)

---

## Requirements

- [beets](https://beets.readthedocs.io) installed and configured
- [ffmpeg](https://ffmpeg.org/download.html) (`ffprobe`) — required for bitrate detection in `upgrades`

## Getting started

### 1. Install

```sh
go install github.com/dewey/beets-importer@latest
```

Or build from source:

```sh
make build
```

### 2. Create your config file

```sh
beets-importer config
```

This copies [`internal/config/config.example.yaml`](internal/config/config.example.yaml) to `~/.config/beets-importer/config.yaml` and prints the path.

### 3. Edit the config file

Open the file and fill in at minimum the three required fields:

```yaml
# Path to your beets SQLite database
db: ~/Music/Music Library Beets/Library/musiclibrary.db

# Directory to scan for new albums and upgrade candidates
source: ~/Music/Music Library Beets/Inbox

# Path to the beet binary or wrapper script
beet: /opt/homebrew/bin/beet
```

The file contains comments explaining every option, including which are required and which are optional.

### 4. Verify

```sh
beets-importer config
```

### 5. Run

```sh
# Pick from recently added albums and import interactively
beets-importer import --latest

# Find albums in your source folder that are higher quality than your library copy
beets-importer upgrades
```

---

## Configuration

Config file location: `~/.config/beets-importer/config.yaml`

Use `--config /path/to/file.yaml` to load a different file.

### Beet wrapper script

If you manage beets with [uv](https://docs.astral.sh/uv/) and a project-local config, point `beet` at a wrapper script instead of the binary directly. For example, `~/Music/Music Library Beets/beet.sh`:

```bash
#!/bin/bash
DIR="$(dirname "$(realpath "$0")")"
exec uv run --project "$DIR" beet -c "$DIR/plugins/config.yaml" "$@"
```

## Global flags

These apply to every subcommand. All path flags have no built-in default — they must be set in the config file or passed explicitly.

| Flag | Description |
|---|---|
| `--db` | Path to beets SQLite database |
| `--source` | Source music directory to scan |
| `--beet` | Path to beet binary or wrapper script |
| `--log` | Path to beets import log (optional) |
| `--verbose` | Print per-directory warnings during scanning instead of a summary count |
| `--no-cache` | Disable the source directory scan cache and force a full re-scan |
| `--config` | Path to config file (default: `~/.config/beets-importer/config.yaml`) |

## Scan cache

To avoid re-reading audio tags and re-running `ffprobe` on every invocation, scan results are cached on disk. Each entry is keyed by path and modification time, so it is automatically invalidated when files change.

| OS | Path |
|---|---|
| macOS | `~/Library/Caches/beets-importer/scan-cache.json` |
| Linux | `~/.cache/beets-importer/scan-cache.json` |

The file is plain JSON and can be deleted at any time — the next run rebuilds it. Pass `--no-cache` to skip it for a single run.

---

## `upgrades` — find upgrade candidates

Scans the source directory and compares each album against your beets library. Reports albums where the source copy is higher quality: a format upgrade (e.g. FLAC replacing MP3) or a meaningfully higher bitrate for the same format.

Matching uses fuzzy comparison on artist + album name. Tags embedded in the first audio file take priority over the directory name. The confidence score weights artist (35%), album name (55%), and release year (10% when both sides have a known year). Pairs where both years are known and differ by more than 3 years are not considered the same album.

### Example commands

```sh
# Show a table of all upgrade candidates in the source directory
beets-importer upgrades

# Limit to the first 10 candidates found (useful for a quick check)
beets-importer upgrades --limit 10

# Only find candidates where the library copy is MP3 or AAC (skip FLAC libraries)
beets-importer upgrades --library-format MP3,AAC

# Only find candidates where the source copy is FLAC (lossless upgrades only)
beets-importer upgrades --source-format FLAC

# Combine filters: library is MP3/AAC and source is FLAC, open an interactive picker
beets-importer upgrades --library-format MP3,AAC --source-format FLAC --pick

# Write results to CSV, then import from it later
beets-importer upgrades --output-file /tmp/upgrades.csv
beets-importer import --from-file /tmp/upgrades.csv
```

### `--pick` — interactive picker

When `--pick` is passed, an interactive picker opens after scanning instead of printing a table. Each candidate is shown as two aligned rows so you can compare source and library side by side:

```
  [ ] Source:   Blumentopf          Kein Zufall             1999  16   FLAC   Similarity 0.90
      Library:  Blumentopf          Kein Zufall             1999  16   MP3    FLAC replaces MP3 (192kbps)
  [ ] Source:   Curren$y            Pilot Talk II           2010  6    FLAC   Similarity 0.86
      Library:  Curren$y            Pilot Talk III          2015  14   MP3    FLAC replaces MP3 (256kbps)
```

Columns: artist · album · year · track count · format. Fields that match are highlighted green; mismatches (year, track count) are highlighted orange.

**Controls:** `SPACE` toggle · `CTRL+A` select all · `j/k` or arrows navigate · `ENTER` confirm · `ESC` cancel · `I` inspect

### Flags

| Flag | Default | Description |
|---|---|---|
| `--pick` | false | Open an interactive picker after scanning to select candidates to import |
| `--limit` | 0 | Stop after finding this many candidates (0 = scan everything) |
| `--threshold` | 0.70 | Minimum similarity score (0–1) to consider a source/library pair a match |
| `--min-bitrate-delta` | 32 | Minimum bitrate improvement in kbps to flag a same-format upgrade |
| `--library-format` | — | Only consider library albums in these formats, comma-separated (e.g. `MP3,AAC`) |
| `--source-format` | — | Only consider source albums in these formats, comma-separated (e.g. `FLAC`) |
| `--require-year-match` | false | Skip candidates where both sides have a known year that differs |
| `--output-file` | — | Write candidates to a CSV file instead of printing a table |

### Output table columns

| Column | Description |
|---|---|
| Source Directory | Directory name of the source album |
| Library Match | Matched beets library entry (`Artist / Album (Year)`) |
| Score | Similarity 0–1 (artist 35% + album 55% + year 10% when both known) |
| Year | `✓` when both sides share the same release year, `✗` when they differ |
| Format | Format change, e.g. `MP3→FLAC` |
| Avg Bitrate | Bitrate change, e.g. `192→870 kbps` |
| Upgrade Reason | e.g. `FLAC replaces MP3 (192kbps)` or `385kbps replaces 320kbps` |

---

## `import` — import albums

Two modes, selected by a required flag.

### `--latest` — interactive picker

Lists recently-added directories in the source folder that aren't already in your beets library, sorted newest-first. Opens an interactive picker to select what to import, then calls `beet import` for each selection.

```sh
# Pick from everything new in the source directory
beets-importer import --latest

# Only show the 20 most recently added
beets-importer import --latest --limit 20

# Only show albums added after a specific date
beets-importer import --latest --after 2024-01-01
```

An album is considered already imported if it appears in the beets import log (`--log`) or if the library contains a high-confidence match (controlled by `--threshold`).

### `--from-file` — import from a text file

Calls `beet import` for each path in a file. Two formats are supported, detected by file extension:

- **`.csv`** — reads the `source_path` column; the CSV produced by `upgrades --output-file` works directly
- **anything else** — one path per line; lines starting with `#` and blank lines are ignored

```sh
beets-importer import --from-file /tmp/upgrades.csv
beets-importer import --from-file /tmp/my-list.txt --limit 5
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--latest` | false | Pick from recently-added unimported albums (mutually exclusive with `--from-file`) |
| `--from-file` | — | File of album paths to import (mutually exclusive with `--latest`) |
| `--limit` | 0 | Maximum number of albums to process (0 = no limit) |
| `--after` | — | Only show albums added after this date (YYYY-MM-DD); `--latest` only |
| `--threshold` | 0.85 | Similarity above which an album is considered already imported; `--latest` only |
