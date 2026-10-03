package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dewey/beets-importer/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Create or show the config file",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write an example config file to the config path",
	Args:  cobra.NoArgs,
	RunE:  runConfigInit,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the values loaded from the config file",
	Args:  cobra.NoArgs,
	RunE:  runConfigShow,
}

func init() {
	configCmd.AddCommand(configInitCmd, configShowCmd)
}

func runConfigInit(_ *cobra.Command, _ []string) error {
	if err := config.WriteTemplate(loadedConfigPath); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Wrote example config to:\n\n  %s\n\n", loadedConfigPath)
	fmt.Fprintf(os.Stderr, "Fill in the required fields, then run 'beets-importer config show' to check it.\n")
	return nil
}

type configRow struct {
	key      string
	val      string
	required bool
	// fromDefault marks a value that is not in the config file.
	fromDefault bool
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	if !loadedConfigFound {
		return fmt.Errorf("no config file at %s: run 'beets-importer config init' to create one", loadedConfigPath)
	}
	cfg := loadedConfig

	// Resolve the values like a command does, so defaults from beets show up.
	fs := pflag.NewFlagSet("show", pflag.ContinueOnError)
	for _, n := range []string{"db", "source", "beet", "state-file", "data-dir", "output"} {
		fs.String(n, "", "")
	}
	applyConfigToFlags(cfg, fs)
	if err := applyBeetsDefaults(fs); err != nil {
		return err
	}
	get := func(n string) string { v, _ := fs.GetString(n); return v }
	dataDir, output := get("data-dir"), get("output")
	if dataDir == "" && get("db") != "" {
		dataDir = filepath.Join(filepath.Dir(get("db")), "beets-importer")
	}
	if output == "" && dataDir != "" {
		output = filepath.Join(dataDir, "report")
	}
	rows := []configRow{
		{"source", cfg.Source, true, false},
		{"db", get("db"), false, cfg.DB == ""},
		{"beet", get("beet"), false, cfg.Beet == ""},
		{"state_file", get("state-file"), false, cfg.StateFile == ""},
		{"data_dir", dataDir, false, cfg.DataDir == ""},
		{"report_output", output, false, cfg.ReportOutput == ""},
		{"verbose", fmt.Sprint(cfg.Verbose), false, false},
		{"no_cache", fmt.Sprint(cfg.NoCache), false, false},
	}
	if cfg.Doctor.LowQualityThresholdKbps != 0 {
		rows = append(rows, configRow{key: "doctor.low_quality_threshold_kbps", val: fmt.Sprint(cfg.Doctor.LowQualityThresholdKbps)})
	}
	names := make([]string, 0, len(cfg.Doctor.Linters))
	for n := range cfg.Doctor.Linters {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		rows = append(rows, configRow{key: "doctor.linters." + n, val: fmt.Sprint(cfg.Doctor.Linters[n])})
	}
	if len(cfg.Ignore.Albums) > 0 {
		rows = append(rows, configRow{key: "ignore.albums", val: strings.Join(cfg.Ignore.Albums, ", ")})
	}

	w := os.Stdout
	fmt.Fprintln(w, styleHeader.Render("Configuration"))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s  %s\n\n", styleLabel.Render("config file:"), styleDim.Render(loadedConfigPath))
	for _, r := range rows {
		tag := styleDim.Render("optional")
		if r.required {
			tag = styleFound.Render("required")
		}
		val := r.val
		if val == "" {
			val = styleDim.Render("(not set)")
		} else if r.fromDefault {
			val += styleDim.Render("  (default)")
		}
		fmt.Fprintf(w, "  %s  %-36s  %s\n", tag, styleLabel.Render(r.key), val)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", styleDim.Render("Flags passed on the command line override these values."))
	return nil
}
