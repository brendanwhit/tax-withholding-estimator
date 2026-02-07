.PHONY: build test lint clean run dev dev-watch check

build:
	go build ./...

test:
	go test ./... -v

lint:
	golangci-lint run

clean:
	rm -rf data/ bin/ tmp/

run:
	go run ./cmd/server

dev:
	go run -tags dev ./cmd/server

dev-watch:
	@command -v air >/dev/null 2>&1 || { echo "Installing air..."; go install github.com/air-verse/air@latest; }
	$$(go env GOPATH)/bin/air

check: lint test build
