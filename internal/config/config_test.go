package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dewey/beets-importer/internal/config"
)

// ── ExpandPath ────────────────────────────────────────────────────────────────

func TestExpandPath_noTilde(t *testing.T) {
	in := "/absolute/path"
	if got := config.ExpandPath(in); got != in {
		t.Errorf("ExpandPath(%q) = %q, want unchanged", in, got)
	}
}

func TestExpandPath_tildeSlash(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := config.ExpandPath("~/foo/bar")
	want := filepath.Join(home, "foo", "bar")
	if got != want {
		t.Errorf("ExpandPath(~/foo/bar) = %q, want %q", got, want)
	}
}

func TestExpandPath_tildeOnly(t *testing.T) {
	home, _ := os.UserHomeDir()
	if got := config.ExpandPath("~"); got != home {
		t.Errorf("ExpandPath(~) = %q, want %q", got, home)
	}
}

func TestExpandPath_emptyString(t *testing.T) {
	if got := config.ExpandPath(""); got != "" {
		t.Errorf("ExpandPath('') = %q, want empty", got)
	}
}

// ── Load ──────────────────────────────────────────────────────────────────────

func TestLoad_fileNotFound(t *testing.T) {
	_, found, err := config.Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Error("found should be false for a missing file")
	}
}

func TestLoad_validYAML(t *testing.T) {
	path := writeFile(t, `
db: /some/db.db
source: /some/source
beet: /usr/local/bin/beet
log: /some/import.log
verbose: true
no_cache: true
`)
	cfg, found, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if cfg.DB != "/some/db.db" {
		t.Errorf("DB = %q, want /some/db.db", cfg.DB)
	}
	if cfg.Source != "/some/source" {
		t.Errorf("Source = %q, want /some/source", cfg.Source)
	}
	if cfg.Beet != "/usr/local/bin/beet" {
		t.Errorf("Beet = %q, want /usr/local/bin/beet", cfg.Beet)
	}
	if cfg.Log != "/some/import.log" {
		t.Errorf("Log = %q, want /some/import.log", cfg.Log)
	}
	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
	if !cfg.NoCache {
		t.Error("NoCache should be true")
	}
}

func TestLoad_tildeExpansion(t *testing.T) {
	home, _ := os.UserHomeDir()
	path := writeFile(t, "db: ~/Music/library.db\n")
	cfg, _, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(home, "Music", "library.db")
	if cfg.DB != want {
		t.Errorf("DB = %q, want %q", cfg.DB, want)
	}
}

func TestLoad_emptyFile(t *testing.T) {
	path := writeFile(t, "")
	cfg, found, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for empty (but existing) file")
	}
	if cfg.DB != "" || cfg.Source != "" {
		t.Errorf("expected zero Config for empty file, got %+v", cfg)
	}
}

func TestLoad_partialYAML(t *testing.T) {
	path := writeFile(t, "db: /only/db\n")
	cfg, found, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if cfg.DB != "/only/db" {
		t.Errorf("DB = %q, want /only/db", cfg.DB)
	}
	if cfg.Source != "" {
		t.Errorf("Source should be empty for partial config, got %q", cfg.Source)
	}
}

func TestLoad_corruptYAML(t *testing.T) {
	path := writeFile(t, "not: valid: yaml: :::")
	_, _, err := config.Load(path)
	if err == nil {
		t.Error("expected error for corrupt YAML")
	}
}

func TestLoad_commentedOutValues(t *testing.T) {
	path := writeFile(t, config.Template)
	cfg, found, err := config.Load(path)
	if err != nil {
		t.Fatalf("template parse error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	// All values should be zero — template is fully commented out.
	if cfg.DB != "" || cfg.Source != "" || cfg.Beet != "" || cfg.Log != "" {
		t.Errorf("expected all-zero Config from template, got %+v", cfg)
	}
}

// ── WriteTemplate ─────────────────────────────────────────────────────────────

func TestWriteTemplate_createsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	if err := config.WriteTemplate(path); err != nil {
		t.Fatalf("WriteTemplate: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written template: %v", err)
	}
	if !strings.Contains(string(data), "beets-importer") {
		t.Error("template content looks wrong")
	}
}

func TestWriteTemplate_noopIfExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	existing := "db: /existing\n"
	os.WriteFile(path, []byte(existing), 0o644)

	if err := config.WriteTemplate(path); err != nil {
		t.Fatalf("WriteTemplate: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != existing {
		t.Error("WriteTemplate should not overwrite an existing file")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	return path
}
