.PHONY: build test lint clean run

build:
	go build ./...

test:
	go test ./... -v

lint:
	golangci-lint run

clean:
	rm -rf data/ bin/

run:
	go run ./cmd/server

check: lint test build
