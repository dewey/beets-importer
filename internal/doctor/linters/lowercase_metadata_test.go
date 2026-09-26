package linters_test

import (
	"testing"

	"github.com/dewey/beets-importer/internal/beets"
	"github.com/dewey/beets-importer/internal/doctor/linters"
)

func TestLowercaseMetadataFlagsAllLowercase(t *testing.T) {
	items := []beets.Item{
		{Path: "/music/a.mp3", Artist: "amon tobin", Album: "foley room", Title: "bloodstone"},
		{Path: "/music/b.mp3", Artist: "amon tobin", Album: "Foley Room", Title: "bloodstone"},
		{Path: "/music/c.mp3", Artist: "!!!", Album: "123", Title: "..."},
	}
	issues, err := linters.NewLowercaseMetadata(items).Run(t.Context())
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	assertIssues(t, issues, []string{"/music/a.mp3"})
}
