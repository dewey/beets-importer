package doctor

import (
	"context"
	"sync"
	"time"
)

// Run executes all enabled linters concurrently and streams results over the
// returned channel. The channel is closed once every linter has finished.
// Disabled linters immediately emit a skipped Result so the caller always
// receives exactly len(specs) values.
func Run(ctx context.Context, specs []Spec) <-chan Result {
	ch := make(chan Result, len(specs))
	var wg sync.WaitGroup
	for _, spec := range specs {
		if !spec.Enabled {
			reason := spec.SkipReason
			if reason == "" {
				reason = "disabled"
			}
			ch <- Result{
				LinterName: spec.Linter.Name(),
				LinterDesc: spec.Linter.Description(),
				Skipped:    true,
				SkipReason: reason,
			}
			continue
		}
		wg.Add(1)
		go func(l Linter) {
			defer wg.Done()
			start := time.Now()
			issues, err := l.Run(ctx)
			ch <- Result{
				LinterName: l.Name(),
				LinterDesc: l.Description(),
				Issues:     issues,
				Duration:   time.Since(start),
				Err:        err,
			}
		}(spec.Linter)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}
