package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dewey/beets-importer/internal/config"
	"github.com/spf13/pflag"
)

// newTestFlagSet creates a pflag.FlagSet that mirrors the persistent flags
// registered on rootCmd, but with isolated variables so tests don't interfere
// with the real command tree.
func newTestFlagSet() (pf *pflag.FlagSet, db, source, beet, state *string, verbose, noCache *bool) {
	db = new(string)
	source = new(string)
	beet = new(string)
	state = new(string)
	verbose = new(bool)
	noCache = new(bool)

	pf = pflag.NewFlagSet("test", pflag.ContinueOnError)
	pf.StringVar(db, "db", "", "")
	pf.StringVar(source, "source", "", "")
	pf.StringVar(beet, "beet", "", "")
	pf.StringVar(state, "state-file", "", "")
	pf.BoolVar(verbose, "verbose", false, "")
	pf.BoolVar(noCache, "no-cache", false, "")
	return
}

func TestApplyConfigToFlags_appliesWhenNotChanged(t *testing.T) {
	pf, db, source, beet, state, verbose, noCache := newTestFlagSet()

	cfg := config.Config{
		DB:        "/config/db.db",
		Source:    "/config/source",
		Beet:      "/config/beet",
		StateFile: "/config/state.pickle",
		Verbose:   true,
		NoCache:   true,
	}
	applyConfigToFlags(cfg, pf)

	if *db != "/config/db.db" {
		t.Errorf("db = %q, want /config/db.db", *db)
	}
	if *source != "/config/source" {
		t.Errorf("source = %q, want /config/source", *source)
	}
	if *beet != "/config/beet" {
		t.Errorf("beet = %q, want /config/beet", *beet)
	}
	if *state != "/config/state.pickle" {
		t.Errorf("state = %q, want /config/state.pickle", *state)
	}
	if !*verbose {
		t.Error("verbose should be true")
	}
	if !*noCache {
		t.Error("no-cache should be true")
	}
}

func TestApplyConfigToFlags_cliWinsOverConfig(t *testing.T) {
	pf, db, _, _, _, _, _ := newTestFlagSet()

	// Simulate user passing --db /cli/db on the command line.
	pf.Parse([]string{"--db", "/cli/db"}) //nolint:errcheck

	cfg := config.Config{DB: "/config/db.db"}
	applyConfigToFlags(cfg, pf)

	// CLI value must win.
	if *db != "/cli/db" {
		t.Errorf("db = %q, want /cli/db (CLI should win over config)", *db)
	}
}

func TestApplyConfigToFlags_emptyConfigFieldLeavesDefault(t *testing.T) {
	pf, db, _, _, _, _, _ := newTestFlagSet()
	// Set a pre-existing "default" on the variable directly (not via Parse,
	// so Changed remains false), mimicking the compiled-in default.
	*db = "compiled-default"

	cfg := config.Config{DB: ""} // config has no db value
	applyConfigToFlags(cfg, pf)

	if *db != "compiled-default" {
		t.Errorf("db = %q, want compiled-default (empty config should not overwrite)", *db)
	}
}

func TestApplyConfigToFlags_partialConfig(t *testing.T) {
	pf, db, source, _, _, _, _ := newTestFlagSet()

	cfg := config.Config{DB: "/config/db.db"} // source not set
	applyConfigToFlags(cfg, pf)

	if *db != "/config/db.db" {
		t.Errorf("db = %q, want /config/db.db", *db)
	}
	if *source != "" {
		t.Errorf("source = %q, want empty (not in config)", *source)
	}
}

func TestApplyConfigToFlags_boolFalseInConfigDoesNotOverride(t *testing.T) {
	pf, _, _, _, _, verbose, _ := newTestFlagSet()
	// verbose is already false (the default). Config also says false.
	// The flag should remain false — not spuriously "applied".
	cfg := config.Config{Verbose: false}
	applyConfigToFlags(cfg, pf)

	if *verbose {
		t.Error("verbose should remain false; false in config must not flip it")
	}
}

func TestRequireFlag_missingReturnsError(t *testing.T) {
	loadedConfigPath = "/some/config.yaml" // set for error message
	if err := requireFlag("source", ""); err == nil {
		t.Error("expected error for empty source flag")
	}
}

func TestRequireFlag_presentReturnsNil(t *testing.T) {
	if err := requireFlag("source", "/some/path"); err != nil {
		t.Errorf("unexpected error for set flag: %v", err)
	}
}

func TestApplyConfigToFlagsSkipsMissingFlags(t *testing.T) {
	pf := pflag.NewFlagSet("test", pflag.ContinueOnError)
	db := pf.String("db", "", "")
	applyConfigToFlags(config.Config{DB: "/config/db.db", Source: "/config/source"}, pf)
	if *db != "/config/db.db" {
		t.Errorf("db = %q, want /config/db.db", *db)
	}
}

func TestDataPath(t *testing.T) {
	defer func(db, dir string) { flagDB, flagDataDir = db, dir }(flagDB, flagDataDir)

	flagDB, flagDataDir = "/music/Library/lib.db", ""
	got, err := dataPath("store.db")
	if err != nil || got != "/music/Library/beets-importer/store.db" {
		t.Errorf("default = %q, %v", got, err)
	}

	flagDataDir = "/data"
	if got, _ = dataPath("store.db"); got != "/data/store.db" {
		t.Errorf("explicit = %q", got)
	}

	flagDB, flagDataDir = "", ""
	if _, err = dataPath("store.db"); err == nil {
		t.Error("want error without db and data dir")
	}
}

func fakeBeet(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$2\" = -p ]; then echo " + dir + "/config.yaml; else printf 'library: lib.db\\nstatefile: state.pickle\\n'; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "beet"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestApplyBeetsDefaults(t *testing.T) {
	dir := fakeBeet(t)
	t.Setenv("PATH", dir)
	pf, db, _, beet, state, _, _ := newTestFlagSet()

	if err := applyBeetsDefaults(pf); err != nil {
		t.Fatal(err)
	}
	if *beet != filepath.Join(dir, "beet") || *db != filepath.Join(dir, "lib.db") || *state != filepath.Join(dir, "state.pickle") {
		t.Errorf("beet=%q db=%q state=%q", *beet, *db, *state)
	}
}

func TestApplyBeetsDefaultsKeepsSetValues(t *testing.T) {
	t.Setenv("PATH", fakeBeet(t))
	pf, db, _, _, _, _, _ := newTestFlagSet()
	if err := pf.Set("db", "/mine.db"); err != nil {
		t.Fatal(err)
	}
	if err := applyBeetsDefaults(pf); err != nil {
		t.Fatal(err)
	}
	if *db != "/mine.db" {
		t.Errorf("db = %q, want /mine.db", *db)
	}
}

func TestApplyBeetsDefaultsNoBeet(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	pf, db, _, beet, _, _, _ := newTestFlagSet()
	if err := applyBeetsDefaults(pf); err != nil {
		t.Fatal(err)
	}
	if *beet != "" || *db != "" {
		t.Errorf("beet=%q db=%q, want both empty", *beet, *db)
	}
}
