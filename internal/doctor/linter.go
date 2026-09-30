package doctor

import (
	"context"
	"time"
)

// Severity classifies how serious an issue is.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is a single problem found by a linter.
type Issue struct {
	Path        string
	AlbumID     int // set by linters that check beets albums, 0 otherwise
	Description string
	Severity    Severity
}

// Result is the outcome of running one linter.
type Result struct {
	LinterName string
	LinterDesc string
	Issues     []Issue
	Duration   time.Duration
	Skipped    bool
	SkipReason string // set when Skipped=true
	Err        error
}

// Linter is the interface implemented by every health check.
type Linter interface {
	Name() string
	Description() string
	Run(ctx context.Context) ([]Issue, error)
}

// Spec pairs a linter with whether it should run.
type Spec struct {
	Linter     Linter
	Enabled    bool
	SkipReason string // shown in the UI when Enabled=false
}
