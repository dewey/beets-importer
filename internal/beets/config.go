package beets

import (
	"fmt"
	"os/exec"

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
