package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dewey/beets-importer/internal/report"
	"github.com/dewey/beets-importer/internal/store"
	"github.com/spf13/cobra"
)

// diskScanInterval is how old the cached file sizes and cover folders may get.
const diskScanInterval = 30 * 24 * time.Hour

var (
	flagReportOutput      string
	flagReportNoSnapshot  bool
	flagReportRefreshDisk bool
)

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "Generate a static HTML report about your beets library",
	Long: `Read the beets database and write index.html to the output folder: format and
bitrate mix, size on disk, release years, artists, labels, genres, metadata
gaps, recent imports and the progress towards an all-lossless library. The page
is one file and needs no server.

File sizes and cover images are read from the music folder, which is slow on a
network share. They are cached in the store in the data folder and read
again in full once a month. Files that are new since the last scan are read on
the next run.

Each run also saves a snapshot to the store, so the report can show progress
over time. Pass --no-snapshot to leave it out.`,
	Args: cobra.NoArgs,
	RunE: runReport,
}

func init() {
	addDBFlag(reportCmd)
	addDataDirFlag(reportCmd)
	reportCmd.Flags().StringVar(&flagReportOutput, "output", "", "Folder to write index.html to (default: 'report' in the data folder)")
	reportCmd.Flags().BoolVar(&flagReportNoSnapshot, "no-snapshot", false, "Do not save a snapshot of this run to the store")
	reportCmd.Flags().BoolVar(&flagReportRefreshDisk, "refresh-disk", false, "Read all file sizes and cover images from disk again, even if the last scan is recent")
}

func runReport(cmd *cobra.Command, _ []string) error {
	if err := requireFlag("db", flagDB); err != nil {
		return err
	}
	if flagReportOutput == "" {
		var err error
		if flagReportOutput, err = dataPath("report"); err != nil {
			return err
		}
	}

	storeFile, err := dataPath("store.db")
	if err != nil {
		return err
	}
	st, err := store.Open(storeFile)
	if err != nil {
		return err
	}
	defer st.Close()

	now := time.Now()
	facts, scannedAt, err := st.DiskFacts()
	if err != nil {
		return err
	}
	full := flagReportRefreshDisk || scannedAt.IsZero() || now.Sub(scannedAt) > diskScanInterval
	if full {
		fmt.Fprintln(os.Stderr, "Reading file sizes and cover images from disk. This can take a while on a network share.")
		facts = store.DiskFacts{}
		scannedAt = now
	}
	disk := report.NewDiskCache(facts.Sizes, facts.Covers)

	data, err := report.Collect(flagDB, loadedConfig.Ignore.Album, now, disk)
	if err != nil {
		return err
	}
	// An unmounted music folder would cache every file as missing for a month.
	if data.Totals.MissingFiles*2 > data.Totals.Tracks {
		return fmt.Errorf("%d of %d files are missing on disk: is the music folder mounted? Nothing was saved", data.Totals.MissingFiles, data.Totals.Tracks)
	}
	data.SizesScanned = scannedAt.Format("2006-01-02")

	sizes, covers := disk.Read()
	if err = st.SaveDiskFacts(store.DiskFacts{Sizes: sizes, Covers: covers}, full, now); err != nil {
		return err
	}
	if !flagReportNoSnapshot {
		if err = st.SaveSnapshot(data.Snapshot()); err != nil {
			return err
		}
	}
	if data.History, err = st.Snapshots(); err != nil {
		return err
	}

	if err = os.MkdirAll(flagReportOutput, 0o755); err != nil {
		return fmt.Errorf("create output folder: %w", err)
	}
	out := filepath.Join(flagReportOutput, "index.html")
	f, err := os.Create(out)
	if err != nil {
		return fmt.Errorf("create report file: %w", err)
	}
	defer f.Close()
	if err = report.Render(f, data); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Wrote report to %s\n", out)
	return nil
}
