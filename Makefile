.PHONY: test test-verbose test-coverage test-race build clean

# Test commands
test:
	go test ./pkg/...

test-verbose:
	go test ./pkg/... -v

test-coverage:
	go test ./pkg/... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

test-race:
	go test ./pkg/... -race

# Build commands
build:
	go build -o bin/nugs-downloader .

build-all:
	GOOS=linux GOARCH=amd64 go build -o bin/nugs-downloader-linux-amd64 .
	GOOS=windows GOARCH=amd64 go build -o bin/nugs-downloader-windows-amd64.exe .
	GOOS=darwin GOARCH=amd64 go build -o bin/nugs-downloader-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o bin/nugs-downloader-darwin-arm64 .

# Development
clean:
	go clean
	rm -f coverage.out coverage.html
	rm -rf bin/

# CI/CD
ci-test:
	go test ./pkg/... -v -race -coverprofile=coverage.out

# Linting (requires golangci-lint)
lint:
	golangci-lint run

# Dependencies
deps:
	go mod download
	go mod tidy
