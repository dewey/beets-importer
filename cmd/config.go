package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/dewey/beets-importer/internal/config"
	"github.com/spf13/cobra"
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
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	if !loadedConfigFound {
		return fmt.Errorf("no config file at %s: run 'beets-importer config init' to create one", loadedConfigPath)
	}
	cfg := loadedConfig
	rows := []configRow{
		{"db", cfg.DB, true},
		{"source", cfg.Source, true},
		{"beet", cfg.Beet, true},
		{"import_log", cfg.ImportLog, false},
		{"verbose", fmt.Sprint(cfg.Verbose), false},
		{"no_cache", fmt.Sprint(cfg.NoCache), false},
	}
	if cfg.Doctor.LowQualityThresholdKbps != 0 {
		rows = append(rows, configRow{"doctor.low_quality_threshold_kbps", fmt.Sprint(cfg.Doctor.LowQualityThresholdKbps), false})
	}
	names := make([]string, 0, len(cfg.Doctor.Linters))
	for n := range cfg.Doctor.Linters {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		rows = append(rows, configRow{"doctor.linters." + n, fmt.Sprint(cfg.Doctor.Linters[n]), false})
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
		}
		fmt.Fprintf(w, "  %s  %-36s  %s\n", tag, styleLabel.Render(r.key), val)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", styleDim.Render("Flags passed on the command line override these values."))
	return nil
}
