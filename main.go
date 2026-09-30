package main

import "github.com/dewey/beets-importer/cmd"

// version is set by goreleaser at build time.
var version = "dev"

func main() {
	cmd.Execute(version)
}
