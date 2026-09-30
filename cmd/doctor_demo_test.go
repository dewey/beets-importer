package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dewey/beets-importer/internal/doctor"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

// TestDoctorEmptyDirsDemo runs the real empty_dirs linter against a real
// directory and renders the result exactly as the doctor TUI would, so we can
// eyeball the output format. Point it at a path via DOCTOR_DEMO_ROOT.
func TestDoctorEmptyDirsDemo(t *testing.T) {
	root := os.Getenv("DOCTOR_DEMO_ROOT")
	if root == "" {
		t.Skip("set DOCTOR_DEMO_ROOT to a library path to run the demo")
	}

	start := time.Now()
	issues, err := linters.NewEmptyDirs(root).Run(t.Context())
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("linter error: %v", err)
	}

	results := []doctor.Result{{
		LinterName: "empty_dirs",
		LinterDesc: "Empty Directories",
		Issues:     issues,
		Duration:   dur,
	}}
	specs := []doctor.Spec{{Linter: linters.NewEmptyDirs(root), Enabled: true}}

	const w = 100
	sep := strings.Repeat("─", w)
	out := fmt.Sprintf("Beets Library Health Report\n%s\n%s\n%s\n%s\n",
		sep,
		renderDoctorResults(results, specs, w),
		sep,
		renderDoctorFooter(results, w),
	)
	fmt.Printf("\n===== root: %s (%d empty dirs) =====\n%s\n", root, len(issues), out)
}
