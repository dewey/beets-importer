package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/dewey/beets-importer/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Persistent flags shared across subcommands.
var (
	flagDB      string
	flagSource  string
	flagBeet    string
	flagLog     string
	flagVerbose bool
	flagNoCache bool
	flagConfig  string
)

// loadedConfigPath and loadedConfigFound are set by PersistentPreRunE so
// subcommands and the config subcommand can reference them.
var (
	loadedConfigPath  string
	loadedConfigFound bool
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

		if found {
			applyConfigToFlags(cfg, cmd.Root().PersistentFlags())
		}
		return nil
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// No default values for path flags — users must configure them explicitly
	// via --flag or the config file. This prevents silent use of wrong paths.
	rootCmd.PersistentFlags().StringVar(&flagDB, "db", "",
		"Path to beets SQLite database")
	rootCmd.PersistentFlags().StringVar(&flagSource, "source", "",
		"Source music directory to scan")
	rootCmd.PersistentFlags().StringVar(&flagBeet, "beet", "",
		"Path to beet binary or wrapper script")
	rootCmd.PersistentFlags().StringVar(&flagLog, "log", "",
		"Path to beets import log")
	rootCmd.PersistentFlags().BoolVar(&flagVerbose, "verbose", false,
		"Print detailed warnings during scanning")
	rootCmd.PersistentFlags().BoolVar(&flagNoCache, "no-cache", false,
		"Disable the source directory scan cache (forces a full re-scan)")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", "",
		"Path to config file (default: ~/.config/beets-importer/config.yaml)")

	rootCmd.AddCommand(upgradesCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(configCmd)
}

// applyConfigToFlags writes config file values into any persistent flags that
// the user did not explicitly set on the command line. Using pf.Set() keeps the
// flag's bound variable in sync and makes this function unit-testable with a
// standalone pflag.FlagSet.
func applyConfigToFlags(cfg config.Config, pf *pflag.FlagSet) {
	for _, f := range []struct{ name, val string }{
		{"db", cfg.DB},
		{"source", cfg.Source},
		{"beet", cfg.Beet},
		{"log", cfg.Log},
	} {
		if f.val != "" && !pf.Changed(f.name) {
			_ = pf.Set(f.name, f.val)
		}
	}
	if cfg.Verbose && !pf.Changed("verbose") {
		_ = pf.Set("verbose", "true")
	}
	if cfg.NoCache && !pf.Changed("no-cache") {
		_ = pf.Set("no-cache", "true")
	}
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
		"--%s is not configured\n\nSet it with --%s /path/to/... or add it to:\n  %s\nRun 'beets-importer config' to see current settings, or --help for flag documentation",
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

// runBeetImport shells out to beet for a single album path, wiring stdin to
// /dev/tty so beets can prompt interactively.
func runBeetImport(path string) error {
	if _, err := os.Stat(flagBeet); err != nil {
		return fmt.Errorf("beet binary not found at %q: set the correct path with --beet or in the config file", flagBeet)
	}
	tty, err := os.Open("/dev/tty")
	if err != nil {
		tty = os.Stdin
	} else {
		defer tty.Close()
	}
	cmd := exec.Command(flagBeet, "import", path)
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

func padRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}
