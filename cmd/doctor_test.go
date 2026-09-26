package cmd

import (
	"testing"

	"github.com/dewey/beets-importer/internal/config"
)

func TestBuildSpecsDisabledByConfig(t *testing.T) {
	cfg := config.DoctorConfig{Linters: map[string]bool{"missing_year": false, "low_quality": true}}
	for _, s := range buildSpecs(nil, nil, "/lib", "/src", cfg) {
		wantEnabled := s.Linter.Name() != "missing_year"
		if s.Enabled != wantEnabled {
			t.Errorf("%s: enabled = %v, want %v", s.Linter.Name(), s.Enabled, wantEnabled)
		}
	}
}

func TestSelectSpecs(t *testing.T) {
	specs := buildSpecs(nil, nil, "/lib", "/src", config.DoctorConfig{})
	got, err := selectSpecs(specs, []string{"missing_year", "empty_dirs"})
	if err != nil {
		t.Fatalf("selectSpecs() error: %v", err)
	}
	if len(got) != 2 || got[0].Linter.Name() != "empty_dirs" || got[1].Linter.Name() != "missing_year" {
		t.Errorf("got %v, want [empty_dirs missing_year] in spec order", got)
	}
	if _, err := selectSpecs(specs, []string{"nope"}); err == nil {
		t.Error("expected error for unknown linter")
	}
}
