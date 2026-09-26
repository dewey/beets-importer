package linters

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor"
)

// LowQuality flags tracks whose bitrate falls below a configurable threshold
// (default 128 kbps), and AAC files below 256 kbps which are considered
// "old AAC" — typically ripped or downloaded before ~2009 at iTunes-store
// quality or lower.
type LowQuality struct {
	items        []beets.Item
	thresholdBps int // e.g. 128_000
}

func NewLowQuality(items []beets.Item, thresholdBps int) *LowQuality {
	return &LowQuality{items: items, thresholdBps: thresholdBps}
}

func (l *LowQuality) Name() string        { return "low_quality" }
func (l *LowQuality) Description() string { return "Low Quality / Old AAC" }

func (l *LowQuality) Run(ctx context.Context) ([]doctor.Issue, error) {
	var issues []doctor.Issue
	for _, item := range l.items {
		if ctx.Err() != nil {
			break
		}
		if item.Bitrate <= 0 {
			continue
		}
		name := filepath.Base(item.Path)
		switch {
		case item.Bitrate < l.thresholdBps:
			issues = append(issues, doctor.Issue{
				Path:        item.Path,
				Description: fmt.Sprintf("%s: %d kbps %s — below %d kbps threshold", name, item.Bitrate/1000, item.Format, l.thresholdBps/1000),
				Severity:    doctor.SeverityWarning,
			})
		case (item.Format == "AAC" || item.Format == "MP4") && item.Bitrate < 256_000:
			issues = append(issues, doctor.Issue{
				Path:        item.Path,
				Description: fmt.Sprintf("%s: %d kbps AAC — old AAC encoding, consider replacing with FLAC or 256 kbps+ AAC", name, item.Bitrate/1000),
				Severity:    doctor.SeverityWarning,
			})
		}
	}
	return issues, nil
}
