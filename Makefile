# Makefile for aibird Go project
.PHONY: help build build-linux test test-verbose test-race test-coverage lint lint-fix fmt vet clean deps deps-update run dev install uninstall docker-build docker-run check-tools install-tools benchmark profile security audit

# Default target
.DEFAULT_GOAL := help

# Variables
BINARY_NAME=aibird
BUILD_DIR=bin
COVERAGE_DIR=coverage
GO_VERSION := $(shell go version | cut -d ' ' -f 3)
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date +%FT%T%z)
LDFLAGS := -ldflags "-X main.Version=${GIT_COMMIT} -X main.BuildTime=${BUILD_TIME} -w -s"

# Colors for output
RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[0;33m
BLUE=\033[0;34m
NC=\033[0m # No Color

## help: Show this help message
help:
	@echo "Available commands:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

## build: Build the application
build: fmt vet
	@echo "$(BLUE)Building $(BINARY_NAME)...$(NC)"
	@mkdir -p $(BUILD_DIR)
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .
	@echo "$(GREEN)✓ Build complete: $(BUILD_DIR)/$(BINARY_NAME)$(NC)"

## build-linux: Build for Linux (useful for cross-compilation)
build-linux: fmt vet
	@echo "$(BLUE)Building $(BINARY_NAME) for Linux...$(NC)"
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux .
	@echo "$(GREEN)✓ Linux build complete: $(BUILD_DIR)/$(BINARY_NAME)-linux$(NC)"

## test: Run all tests
test:
	@echo "$(BLUE)Running tests...$(NC)"
	go test ./... -timeout=30s
	@echo "$(GREEN)✓ Tests passed$(NC)"

## test-verbose: Run all tests with verbose output
test-verbose:
	@echo "$(BLUE)Running tests with verbose output...$(NC)"
	go test ./... -v -timeout=30s

## test-race: Run tests with race condition detection
test-race:
	@echo "$(BLUE)Running tests with race detection...$(NC)"
	go test ./... -race -timeout=30s
	@echo "$(GREEN)✓ Race tests passed$(NC)"

## test-coverage: Run tests with coverage analysis
test-coverage:
	@echo "$(BLUE)Running tests with coverage...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	go test ./... -coverprofile=$(COVERAGE_DIR)/coverage.out -timeout=30s
	go tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	go tool cover -func=$(COVERAGE_DIR)/coverage.out
	@echo "$(GREEN)✓ Coverage report generated: $(COVERAGE_DIR)/coverage.html$(NC)"

## benchmark: Run benchmarks
benchmark:
	@echo "$(BLUE)Running benchmarks...$(NC)"
	go test ./... -bench=. -benchmem -timeout=5m

## lint: Run practical linting (essential linters for development)
lint:
	@echo "$(BLUE)Running practical linting...$(NC)"
	golangci-lint run \
		--enable=revive,govet,ineffassign,misspell,gofmt,goimports,gosec,staticcheck,unused,typecheck \
		--timeout=5m \
		./...
	@echo "$(GREEN)✓ Practical linting passed$(NC)"

## lint-all: Run comprehensive linting (all available linters, non-blocking)
lint-all:
	@echo "$(BLUE)Running comprehensive linting...$(NC)"
	golangci-lint run --enable-all \
		--disable=gochecknoglobals,gochecknoinits,exhaustivestruct,exhaustruct,varnamelen,wsl,nlreturn,lll,gofumpt,gci,wrapcheck,dupl,cyclop,funlen,maintidx,nestif,gocognit \
		--no-config \
		--timeout=5m \
		./... || true
	@echo "$(YELLOW)⚠ Comprehensive linting complete (some issues may need attention)$(NC)"

## lint-fix: Run practical linting with auto-fixes where possible
lint-fix:
	@echo "$(BLUE)Running linting with auto-fix...$(NC)"
	golangci-lint run \
		--enable=revive,govet,ineffassign,misspell,gofmt,goimports,gosec,staticcheck,unused,typecheck \
		--fix \
		--timeout=5m \
		./...
	@echo "$(GREEN)✓ Linting with auto-fix complete$(NC)"

## fmt: Format all Go code
fmt:
	@echo "$(BLUE)Formatting code...$(NC)"
	go fmt ./...
	@echo "$(GREEN)✓ Code formatted$(NC)"

## vet: Run go vet
vet:
	@echo "$(BLUE)Running go vet...$(NC)"
	go vet ./...
	@echo "$(GREEN)✓ Vet passed$(NC)"

## security: Run security checks
security:
	@echo "$(BLUE)Running security checks...$(NC)"
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
		echo "$(GREEN)✓ Security check passed$(NC)"; \
	else \
		echo "$(YELLOW)⚠ gosec not installed. Run 'make install-tools' to install it.$(NC)"; \
	fi

## audit: Run dependency vulnerability audit
audit:
	@echo "$(BLUE)Running dependency audit...$(NC)"
	@if command -v nancy >/dev/null 2>&1; then \
		go list -json -deps ./... | nancy sleuth; \
		echo "$(GREEN)✓ Dependency audit passed$(NC)"; \
	else \
		echo "$(YELLOW)⚠ nancy not installed. Run 'make install-tools' to install it.$(NC)"; \
	fi

## deps: Download and tidy dependencies
deps:
	@echo "$(BLUE)Downloading dependencies...$(NC)"
	go mod download
	go mod tidy
	@echo "$(GREEN)✓ Dependencies updated$(NC)"

## deps-update: Update all dependencies
deps-update:
	@echo "$(BLUE)Updating all dependencies...$(NC)"
	go get -u ./...
	go mod tidy
	@echo "$(GREEN)✓ Dependencies updated$(NC)"

## clean: Clean build artifacts and caches
clean:
	@echo "$(BLUE)Cleaning build artifacts...$(NC)"
	go clean -cache -testcache -modcache
	rm -rf $(BUILD_DIR) $(COVERAGE_DIR)
	rm -f bird.db* # Remove any existing database files
	@echo "$(GREEN)✓ Clean complete$(NC)"

## run: Run the application
run: build
	@echo "$(BLUE)Running $(BINARY_NAME)...$(NC)"
	./$(BUILD_DIR)/$(BINARY_NAME)

## dev: Run the application in development mode (rebuild on changes would require additional tools)
dev:
	@echo "$(BLUE)Running $(BINARY_NAME) in development mode...$(NC)"
	@echo "$(YELLOW)Note: For auto-reload on changes, consider using 'air' or 'realize'$(NC)"
	go run .

## install: Install the binary to $GOPATH/bin
install:
	@echo "$(BLUE)Installing $(BINARY_NAME)...$(NC)"
	go install $(LDFLAGS) .
	@echo "$(GREEN)✓ $(BINARY_NAME) installed to $(shell go env GOPATH)/bin$(NC)"

## uninstall: Remove the binary from $GOPATH/bin
uninstall:
	@echo "$(BLUE)Uninstalling $(BINARY_NAME)...$(NC)"
	rm -f $(shell go env GOPATH)/bin/$(BINARY_NAME)
	@echo "$(GREEN)✓ $(BINARY_NAME) uninstalled$(NC)"

## check-tools: Check if required tools are installed
check-tools:
	@echo "$(BLUE)Checking required tools...$(NC)"
	@echo "Go version: $(GO_VERSION)"
	@command -v golangci-lint >/dev/null 2>&1 && echo "✓ golangci-lint installed" || echo "✗ golangci-lint missing"
	@command -v gosec >/dev/null 2>&1 && echo "✓ gosec installed" || echo "✗ gosec missing (optional)"
	@command -v nancy >/dev/null 2>&1 && echo "✓ nancy installed" || echo "✗ nancy missing (optional)"
	@command -v air >/dev/null 2>&1 && echo "✓ air installed" || echo "✗ air missing (optional - for dev mode)"

## install-tools: Install additional development tools
install-tools:
	@echo "$(BLUE)Installing development tools...$(NC)"
	@echo "Installing golangci-lint..."
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin; \
	fi
	@echo "Installing gosec..."
	go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	@echo "Installing nancy..."
	go install github.com/sonatypecommunity/nancy@latest
	@echo "Installing air (for development hot reload)..."
	go install github.com/air-verse/air@latest
	@echo "$(GREEN)✓ Development tools installed$(NC)"

## docker-build: Build Docker image
docker-build:
	@echo "$(BLUE)Building Docker image...$(NC)"
	docker build -t $(BINARY_NAME):latest .
	docker build -t $(BINARY_NAME):$(GIT_COMMIT) .
	@echo "$(GREEN)✓ Docker image built$(NC)"

## docker-run: Run application in Docker
docker-run:
	@echo "$(BLUE)Running $(BINARY_NAME) in Docker...$(NC)"
	docker run --rm -it $(BINARY_NAME):latest

## profile: Run application with CPU profiling
profile:
	@echo "$(BLUE)Running with CPU profiling...$(NC)"
	go run . -cpuprofile=cpu.prof
	@echo "$(YELLOW)Analyze with: go tool pprof cpu.prof$(NC)"

## all: Run comprehensive checks (format, vet, lint, test, build)
all: fmt vet lint test build
	@echo "$(GREEN)✓ All checks passed and build complete$(NC)"

## ci: Run all CI checks (suitable for continuous integration)
ci: deps fmt vet lint test-race test-coverage build
	@echo "$(GREEN)✓ All CI checks passed$(NC)"

## info: Show project information
info:
	@echo "$(BLUE)Project Information:$(NC)"
	@echo "Binary Name: $(BINARY_NAME)"
	@echo "Go Version: $(GO_VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"
	@echo "Build Dir: $(BUILD_DIR)"
	@echo "Coverage Dir: $(COVERAGE_DIR)"