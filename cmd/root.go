package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Flags shared by several subcommands. Each subcommand registers only the ones
// it uses, so --help lists only flags that matter.
var (
	flagDB        string
	flagSource    string
	flagBeet      string
	flagStateFile string
	flagDataDir   string
	flagVerbose   bool
	flagNoCache   bool
	flagConfig    string
)

// loadedConfigPath, loadedConfigFound, and loadedConfig are set by
// PersistentPreRunE so subcommands can reference them without re-parsing.
var (
	loadedConfigPath  string
	loadedConfigFound bool
	loadedConfig      config.Config
)

// Shared lipgloss styles used by both subcommands.
var (
	spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	styleSpinner  = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	styleLabel    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleFound    = lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	styleHeader   = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
)

var rootCmd = &cobra.Command{
	Use:   "beets-importer",
	Short: "Import and upgrade your beets music library",
	// Errors from RunE are about the input, not flag syntax, so the usage text is noise.
	SilenceUsage: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Determine config path: explicit --config flag, or the OS default.
		explicitConfig := cmd.Root().PersistentFlags().Changed("config")
		cfgPath := flagConfig
		if cfgPath == "" {
			p, err := config.DefaultPath()
			if err != nil {
				return fmt.Errorf("resolving config path: %w", err)
			}
			cfgPath = p
		}

		if explicitConfig {
			if _, err := os.Stat(cfgPath); err != nil {
				return fmt.Errorf("config file not found: %s", cfgPath)
			}
		}

		cfg, found, err := config.Load(cfgPath)
		if err != nil {
			return err
		}

		loadedConfigPath = cfgPath
		loadedConfigFound = found
		loadedConfig = cfg

		if found {
			applyConfigToFlags(cfg, cmd.Flags())
		}
		return applyBeetsDefaults(cmd.Flags())
	},
}

func Execute(version string) {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "",
		"Path to config file (default: ~/.config/beets-importer/config.yaml)")

	rootCmd.AddCommand(upgradesCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(maintenanceCmd)
	rootCmd.AddCommand(reportCmd)
}

// Path flags have no defaults, so a wrong path is never used silently. They
// must come from the config file or the command line.
func addDBFlag(c *cobra.Command) {
	c.Flags().StringVar(&flagDB, "db", "", "Path to beets SQLite database")
}

func addSourceFlag(c *cobra.Command) {
	c.Flags().StringVar(&flagSource, "source", "", "Source music directory to scan")
}

func addDataDirFlag(c *cobra.Command) {
	c.Flags().StringVar(&flagDataDir, "data-dir", "", "Folder for the files beets-importer keeps (default: 'beets-importer' next to the beets database)")
}

// dataPath returns the path of a file in the data folder. The folder defaults
// to one next to the beets database, so it follows the library when it moves.
func dataPath(name string) (string, error) {
	dir := flagDataDir
	if dir == "" {
		if err := requireFlag("db", flagDB); err != nil {
			return "", err
		}
		dir = filepath.Join(filepath.Dir(flagDB), "beets-importer")
	}
	return filepath.Join(dir, name), nil
}

func addBeetFlag(c *cobra.Command) {
	c.Flags().StringVar(&flagBeet, "beet", "", "Path to beet binary or wrapper script")
}

func addScanFlags(c *cobra.Command) {
	c.Flags().BoolVar(&flagVerbose, "verbose", false, "Print a warning for every directory that could not be scanned")
	c.Flags().BoolVar(&flagNoCache, "no-cache", false, "Disable the source directory scan cache (forces a full re-scan)")
}

// applyConfigToFlags writes config file values into the flags the user did
// not set on the command line. Flags the command does not have are skipped.
func applyConfigToFlags(cfg config.Config, fs *pflag.FlagSet) {
	vals := []struct{ name, val string }{
		{"db", cfg.DB},
		{"source", cfg.Source},
		{"beet", cfg.Beet},
		{"state-file", cfg.StateFile},
		{"data-dir", cfg.DataDir},
		{"output", cfg.ReportOutput},
		{"verbose", boolFlag(cfg.Verbose)},
		{"no-cache", boolFlag(cfg.NoCache)},
	}
	for _, v := range vals {
		f := fs.Lookup(v.name)
		if f == nil || f.Changed || v.val == "" {
			continue
		}
		_ = fs.Set(v.name, v.val)
	}
}

// applyBeetsDefaults fills the flags still empty from the beets install: the
// beet binary on PATH, then the database and state file beet reports. Nothing
// is guessed: if beet is not found the flags stay empty and requireFlag says so.
func applyBeetsDefaults(fs *pflag.FlagSet) error {
	empty := func(name string) *pflag.Flag {
		if f := fs.Lookup(name); f != nil && f.Value.String() == "" {
			return f
		}
		return nil
	}
	if f := empty("beet"); f != nil {
		if p, err := exec.LookPath("beet"); err == nil {
			_ = f.Value.Set(p)
		}
	}
	db, state := empty("db"), empty("state-file")
	beetFlag := fs.Lookup("beet")
	if (db == nil && state == nil) || beetFlag == nil || beetFlag.Value.String() == "" {
		return nil
	}
	s, err := beets.ReadSettings(beetFlag.Value.String())
	if err != nil {
		return err
	}
	if db != nil {
		_ = db.Value.Set(s.Library)
	}
	if state != nil {
		_ = state.Value.Set(s.StateFile)
	}
	return nil
}

// boolFlag returns "" for false so a false config value never overrides a flag.
func boolFlag(b bool) string {
	if b {
		return "true"
	}
	return ""
}

// requireFlag returns an error if val is empty, with a message pointing the
// user to the config file and --help.
func requireFlag(name, val string) error {
	if val != "" {
		return nil
	}
	hint := loadedConfigPath
	if hint == "" {
		hint = "~/.config/beets-importer/config.yaml"
	}
	return fmt.Errorf(
		"--%s is not configured\n\nSet it with --%s /path/to/... or add it to:\n  %s\nRun 'beets-importer config show' to see current settings, or --help for flag documentation",
		name, name, hint,
	)
}

// startSpinner runs a spinner in a goroutine, reading the current message from msg.
// Returns a stop func that clears the line when called. stop() blocks until the
// goroutine has cleared the line, preventing output interleaving.
func startSpinner(msg *atomic.Value) func() {
	done := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		frame := 0
		for {
			select {
			case <-done:
				fmt.Fprint(os.Stderr, "\r\033[K")
				return
			case <-time.After(80 * time.Millisecond):
				s, _ := msg.Load().(string)
				fmt.Fprintf(os.Stderr, "\r\033[K%s %s",
					styleSpinner.Render(spinnerFrames[frame%len(spinnerFrames)]),
					styleLabel.Render(s),
				)
				frame++
			}
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}

// runBeetImport shells out to 'beet import' with args.
func runBeetImport(args ...string) error {
	return runBeet(append([]string{"import"}, args...)...)
}

// runBeet shells out to beet with args, wiring stdin to /dev/tty so beets
// can prompt interactively.
func runBeet(args ...string) error {
	if _, err := os.Stat(flagBeet); err != nil {
		return fmt.Errorf("beet binary not found at %q: set the correct path with --beet or in the config file", flagBeet)
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		tty = os.Stdin
	} else {
		defer tty.Close()
	}
	cmd := exec.Command(flagBeet, args...)
	cmd.Stdin = tty
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// truncateLeft truncates s from the left, keeping the tail, so that the most
// specific (rightmost) portion of a file path remains visible.
func truncateLeft(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return "…" + string(runes[len(runes)-max+1:])
}

func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}
