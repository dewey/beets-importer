BINARY := beets-importer

.PHONY: build test

build:
	go build -o $(BINARY) .

test:
	go test ./...
