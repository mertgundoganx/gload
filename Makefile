.PHONY: build test lint vet clean run docker release help css licenses

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-X main.Version=$(VERSION)"

## help: Show this help message
help:
	@echo "gload - HTTP Load Testing Tool"
	@echo ""
	@echo "Usage:"
	@echo "  make build       Build the binary"
	@echo "  make test        Run all tests"
	@echo "  make test-v      Run tests with verbose output"
	@echo "  make cover       Run tests with coverage report"
	@echo "  make lint        Run linter (requires golangci-lint)"
	@echo "  make vet         Run go vet"
	@echo "  make clean       Remove build artifacts"
	@echo "  make run         Build and run web server"
	@echo "  make css         Regenerate Tailwind CSS (requires tailwindcss CLI)"
	@echo "  make licenses    Regenerate THIRD_PARTY_LICENSES.md"
	@echo "  make docker      Build Docker image"
	@echo "  make compose     Start full stack (gload + Prometheus + Grafana)"
	@echo "  make compose-down Stop full stack"
	@echo "  make release     Build for all platforms"
	@echo ""

## build: Build the gload binary
build:
	go build $(LDFLAGS) -o gload .

## test: Run all tests
test:
	go test ./... -count=1 -timeout=120s

## test-v: Run tests with verbose output
test-v:
	go test ./... -v -count=1 -timeout=120s

## cover: Run tests with coverage
cover:
	go test ./... -coverprofile=coverage.txt -timeout=120s
	go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run golangci-lint
lint:
	@which golangci-lint > /dev/null 2>&1 || (echo "Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

## vet: Run go vet
vet:
	go vet ./...

## clean: Remove build artifacts
clean:
	rm -f gload coverage.out coverage.html
	rm -f gload-*

## run: Build and run the web server
run: build
	./gload --web

## licenses: Regenerate THIRD_PARTY_LICENSES.md from the module cache
licenses:
	@bash scripts/gen-third-party-licenses.sh

## css: Regenerate the embedded Tailwind CSS from web sources
css:
	@which tailwindcss > /dev/null 2>&1 || (echo "Install the standalone CLI: brew install tailwindcss (or see https://tailwindcss.com/blog/standalone-cli)" && exit 1)
	tailwindcss -i web/tailwind.input.css -o web/static/css/tailwind.css --minify

## docker: Build Docker image
docker:
	docker build -t gload:$(VERSION) -t gload:latest .

## compose: Start full stack
compose:
	docker compose up -d --build

## compose-down: Stop full stack
compose-down:
	docker compose down

## release: Cross-compile for all platforms
release: clean
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o gload-linux-amd64 .
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o gload-linux-arm64 .
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o gload-darwin-amd64 .
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o gload-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o gload-windows-amd64.exe .
	@echo "Release binaries built:"
	@ls -la gload-*
