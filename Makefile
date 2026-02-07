.PHONY: build test lint clean run dev check

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

dev:
	go run -tags dev ./cmd/server

check: lint test build
