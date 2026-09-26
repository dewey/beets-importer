# beets-importer

A CLI tool for [beets](https://beets.readthedocs.io) that helps with two things:

- **`import`**: pick recently-added albums from your download folder and import them interactively, or import from a text file of paths
- **`upgrades`**: scan your download folder, match albums against your library, and surface upgrade candidates: better format (e.g. FLAC replacing MP3) or higher bitrate
- **`doctor`**: check your beets library for problems such as empty or untracked folders, low bitrate files, and missing artwork or years

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

**Homebrew (macOS/Linux):**

```sh
brew tap dewey/beets-importer https://github.com/dewey/beets-importer
brew install dewey/beets-importer/beets-importer
```

**Go:**

```sh
go install github.com/dewey/beets-importer@latest
```

**From source:**

```sh
make build
```

### 2. Create your config file

```sh
beets-importer config init
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
beets-importer config show
```

### 5. Run

```sh
# Pick from recently added albums and import interactively
beets-importer import

# Find albums in your source folder that are higher quality than your library copy
beets-importer upgrades

# Check the health of your library
beets-importer doctor
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

## Shared flags

Each command only accepts the flags it uses. Path flags have no built-in default: set them in the config file or pass them on the command line.

| Flag | Config key | Commands | Description |
|---|---|---|---|
| `--config` | | all | Path to config file (default: `~/.config/beets-importer/config.yaml`) |
| `--db` | `db` | upgrades, doctor | Path to beets SQLite database |
| `--source` | `source` | import, upgrades, doctor | Download folder to scan for new albums |
| `--beet` | `beet` | import, upgrades, doctor | Path to beet binary or wrapper script |
| `--state-file` | `state_file` | import | Path to beets incremental state file (`state.pickle`) |
| `--verbose` | `verbose` | import, upgrades | Print per-directory warnings during scanning instead of a summary count |
| `--no-cache` | `no_cache` | import, upgrades | Disable the source directory scan cache and force a full re-scan |

`beets-importer --version` prints the installed version.

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
beets-importer upgrades --library-format MP3,AAC --source-format FLAC -i

# Write results to CSV, then import from it later
beets-importer upgrades -o /tmp/upgrades.csv
beets-importer import --from-file /tmp/upgrades.csv
```

### `--interactive` / `-i`: interactive picker

When `--interactive` is passed, an interactive picker opens after scanning instead of printing a table. Each candidate is shown as two aligned rows so you can compare source and library side by side:

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
| `--interactive`, `-i` | false | Open an interactive picker after scanning to select candidates to import |
| `--limit` | 0 | Stop after finding this many candidates (0 = scan everything) |
| `--threshold` | 0.70 | Minimum similarity score (0–1) to consider a source/library pair a match |
| `--min-bitrate-delta` | 32 | Minimum bitrate improvement in kbps to flag a same-format upgrade |
| `--library-format` | — | Only consider library albums in these formats, comma-separated (e.g. `MP3,AAC`) |
| `--source-format` | — | Only consider source albums in these formats, comma-separated (e.g. `FLAC`) |
| `--require-year-match` | false | Skip candidates where both sides have a known year that differs |
| `--output`, `-o` | — | Write candidates to a CSV file instead of printing a table |
| `--all` | false | Show all matched pairs, not only upgrade candidates |

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

By default `import` opens a picker. Pass `--from-file` to import a list of paths instead.

### Interactive picker (default)

Lists recently-added directories in the source folder that aren't already in your beets library, sorted newest-first. Opens an interactive picker to select what to import, then calls `beet import` for each selection.

```sh
# Pick from everything new in the source directory
beets-importer import

# Only show the 20 most recently added
beets-importer import --limit 20

# Only show albums added after a specific date
beets-importer import --since 2024-01-01
```

Folders that beets has already processed (applied or skipped) are left out. The list comes from the beets incremental state file (`--state-file`), so beets must run with `incremental: yes`. Paths are compared exactly, so running `import --limit 5` twice shows the next 5 folders once you processed the first ones. Pass `--reimport` to list those folders too. It also runs `beet import --noincremental`, because beets would skip them otherwise.

### `--from-file`: import from a text file

Calls `beet import` for each path in a file. Two formats are supported, detected by file extension:

- **`.csv`** — reads the `source_path` column; the CSV produced by `upgrades --output` works directly
- **anything else** — one path per line; lines starting with `#` and blank lines are ignored

```sh
beets-importer import --from-file /tmp/upgrades.csv
beets-importer import --from-file /tmp/my-list.txt --limit 5
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--from-file` | — | Import the paths in this file instead of opening the picker |
| `--limit` | 0 | Maximum number of albums to process (0 = no limit) |
| `--since` | — | Only show albums added on or after this date (YYYY-MM-DD); not with `--from-file` |
| `--reimport` | false | Also list folders beets already processed and import them again with `--noincremental` |

---

## `doctor`: check library health

Runs a set of linters against your beets library and shows the results in a scrollable view.

`doctor` reads the beets database (`--db`). Some linters also walk the folder beets moves music into. That folder is read from `directory:` in your beets config by running `beet config`, so there is nothing extra to set.

| Linter | Needs library | Flags |
|---|---|---|
| `empty_dirs` | yes | Folders with no entries at all |
| `untracked_dirs` | yes | Folders with audio files but no track in the beets database |
| `low_quality` | no | Tracks below `doctor.low_quality_threshold_kbps` (default 128), and AAC below 256 kbps |
| `lowercase_metadata` | no | Tracks where artist, album and title are all lowercase |
| `missing_artwork` | no | Albums without an art path |
| `missing_year` | no | Albums without a year |
| `duplicate_names` | yes | Folders holding two entries with the same name in different Unicode forms (NFC and NFD). Also walks `--source` |

All linters run by default. Turn one off in the config:

```yaml
doctor:
  linters:
    lowercase_metadata: false
```

### Example commands

```sh
# Interactive report
beets-importer doctor

# Full report as JSON
beets-importer doctor --json

# Only some linters
beets-importer doctor --linter empty_dirs,untracked_dirs

# Paths for one linter, e.g. to remove empty folders
beets-importer doctor --linter empty_dirs --paths --print0 | xargs -0 rmdir
```

### Duplicate Unicode names

A name like "gehört" can be stored with "ö" as one code point (NFC) or as "o" plus a combining mark (NFD). macOS used to write NFD. Linux tools such as torrent clients write NFC. If a Linux tool writes a file next to an NFD copy from a Mac, the folder ends up with both. Over SMB, macOS lists both entries but can only open one of them, so beets reports extra "unmatched tracks".

`duplicate_names` finds these folders. The fix has to run on the file server, where the two names are really different. The Mac cannot tell which copy it is deleting.

### Flags

| Flag | Default | Description |
|---|---|---|
| `--json` | false | Print results as JSON instead of the interactive view |
| `--linter` | — | Run only these linters, comma-separated |
| `--paths` | false | Print only issue paths of the selected linters, one per line (requires `--linter`) |
| `--print0`, `-0` | false | With `--paths`, separate paths with NUL (for `xargs -0`) |
