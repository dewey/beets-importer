package beets

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dewey/beets-importer/internal/config"
	"go.yaml.in/yaml/v3"
)

// LibraryDir asks beet for its music directory, so the path is only
// configured in one place.
func LibraryDir(beetPath string) (string, error) {
	out, err := exec.Command(beetPath, "config").Output()
	if err != nil {
		return "", fmt.Errorf("run %s config: %w", beetPath, err)
	}
	var cfg struct {
		Directory string `yaml:"directory"`
	}
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		return "", fmt.Errorf("parse beet config: %w", err)
	}
	if cfg.Directory == "" {
		return "", fmt.Errorf("beet config has no directory set")
	}
	return config.ExpandPath(cfg.Directory), nil
}

// Settings are the beets options beets-importer can use as defaults.
type Settings struct {
	Library   string
	StateFile string
}

// ReadSettings asks beet for its database and state file, so a standard beets
// install needs no extra setup. Relative paths are relative to the beets
// config folder.
func ReadSettings(beetPath string) (Settings, error) {
	out, err := exec.Command(beetPath, "config", "-d").Output()
	if err != nil {
		return Settings{}, fmt.Errorf("run %s config -d: %w", beetPath, err)
	}
	var cfg struct {
		Library   string `yaml:"library"`
		StateFile string `yaml:"statefile"`
	}
	if err = yaml.Unmarshal(out, &cfg); err != nil {
		return Settings{}, fmt.Errorf("parse beet config: %w", err)
	}
	paths, err := exec.Command(beetPath, "config", "-p").Output()
	if err != nil {
		return Settings{}, fmt.Errorf("run %s config -p: %w", beetPath, err)
	}
	dir := filepath.Dir(strings.SplitN(strings.TrimSpace(string(paths)), "\n", 2)[0])
	resolve := func(p string) string {
		p = config.ExpandPath(p)
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(dir, p)
	}
	return Settings{Library: resolve(cfg.Library), StateFile: resolve(cfg.StateFile)}, nil
}
