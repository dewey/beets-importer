package cmd

import (
	"fmt"
	"os"

	"github.com/dewey/beets-importer/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the effective configuration, creating a template if none exists",
	RunE:  runConfig,
}

func runConfig(_ *cobra.Command, _ []string) error {
	// First run: config file doesn't exist yet — create a template and tell
	// the user to edit it. Don't show the (empty) values table; it would only
	// add noise before anything is configured.
	if !loadedConfigFound {
		if err := config.WriteTemplate(loadedConfigPath); err != nil {
			return fmt.Errorf("creating config template: %w", err)
		}
		fmt.Fprintf(os.Stderr, "%s\n\n", styleHeader.Render("Welcome to beets-importer"))
		fmt.Fprintf(os.Stderr, "No config file found. Copied example config to:\n\n")
		fmt.Fprintf(os.Stderr, "  %s\n\n", loadedConfigPath)
		fmt.Fprintf(os.Stderr, "Open that file, fill in your paths, then re-run this command to verify.\n")
		fmt.Fprintf(os.Stderr, "Required fields are marked in the comments.\n")
		return nil
	}

	w := os.Stdout
	fmt.Fprintln(w, styleHeader.Render("Effective Configuration"))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s  %s\n\n",
		styleLabel.Render("config file:"),
		styleDim.Render(loadedConfigPath),
	)

	type row struct {
		flag     string
		val      string
		required bool
	}
	rows := []row{
		{"db", flagDB, true},
		{"source", flagSource, true},
		{"beet", flagBeet, true},
		{"log", flagLog, false},
		{"verbose", fmt.Sprintf("%v", flagVerbose), false},
		{"no-cache", fmt.Sprintf("%v", flagNoCache), false},
	}

	for _, r := range rows {
		tag := styleDim.Render("optional")
		if r.required {
			tag = styleFound.Render("required")
		}
		val := r.val
		if val == "" {
			val = styleDim.Render("(not set)")
		}
		fmt.Fprintf(w, "  %s  %-16s  %s\n",
			tag,
			styleLabel.Render("--"+r.flag),
			val,
		)
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n",
		styleDim.Render("Edit the config file above to change persistent values, or pass --flags directly."),
	)
	return nil
}
