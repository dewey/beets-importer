package config

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

//go:embed config.example.yaml
var templateBytes []byte

// Template is the content of config.example.yaml, embedded at build time.
// Exported so tests can verify the template parses as valid YAML.
var Template = string(templateBytes)

// DoctorConfig holds settings for the doctor command.
type DoctorConfig struct {
	// LowQualityThresholdKbps is the bitrate below which a track is flagged as
	// low quality. Defaults to 128 kbps when unset.
	LowQualityThresholdKbps int `yaml:"low_quality_threshold_kbps"`
	// Linters turns individual linters off by name (e.g. "empty_dirs": false).
	Linters map[string]bool `yaml:"linters"`
}

// Config holds settings loaded from the config file.
// Keys use underscores to follow YAML convention; flag names use hyphens.
type Config struct {
	DB        string       `yaml:"db"`
	Source    string       `yaml:"source"`
	Beet      string       `yaml:"beet"`
	StateFile string       `yaml:"state_file"`
	Verbose   bool         `yaml:"verbose"`
	NoCache   bool         `yaml:"no_cache"`
	Doctor    DoctorConfig `yaml:"doctor"`
}

// DefaultPath returns the default config file location for the current OS.
// On all platforms this is ~/.config/beets-importer/config.yaml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("user home dir: %w", err)
	}
	return filepath.Join(home, ".config", "beets-importer", "config.yaml"), nil
}

// Load reads and parses the config file at path. If the file does not exist,
// a zero Config and found=false are returned without error. Any other read or
// parse error is returned.
func Load(path string) (cfg Config, found bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, false, nil
		}
		return Config{}, false, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, true, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.DB = ExpandPath(cfg.DB)
	cfg.Source = ExpandPath(cfg.Source)
	cfg.Beet = ExpandPath(cfg.Beet)
	cfg.StateFile = ExpandPath(cfg.StateFile)
	return cfg, true, nil
}

// ExpandPath expands a leading ~ to the user's home directory.
func ExpandPath(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

// WriteTemplate writes a commented-out template config to path, creating
// parent directories as needed. It is a no-op if the file already exists.
func WriteTemplate(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config file already exists: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err := os.WriteFile(path, templateBytes, 0o644); err != nil {
		return fmt.Errorf("write config template: %w", err)
	}
	return nil
}
