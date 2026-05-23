# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
BINARY_NAME=simple-captcha
BINARY_UNIX=$(BINARY_NAME)_unix

# Build the project
build:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...

# Clean build files
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)

# Run tests
test:
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out

# Run benchmarks
benchmark:
	$(GOTEST) -bench=. -benchmem ./...

# Cross compilation for Linux
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v

# Install dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Run the application
run:
	$(GOBUILD) -o $(BINARY_NAME) -v ./...
	./$(BINARY_NAME)

# Run with hot reload (requires air)
dev:
	air

# Format code
fmt:
	$(GOCMD) fmt ./...

# Lint code (requires golangci-lint)
lint:
	golangci-lint run

# Security check (requires gosec)
security:
	gosec ./...

# Generate documentation
docs:
	godoc -http=:6060

# Docker build
docker-build:
	docker build -t $(BINARY_NAME) .

# Docker run
docker-run:
	docker run -p 8080:8080 $(BINARY_NAME)

# Help
help:
	@echo "Available commands:"
	@echo "  build         - Build the project"
	@echo "  clean         - Clean build files"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage"
	@echo "  benchmark     - Run benchmarks"
	@echo "  build-linux   - Cross compile for Linux"
	@echo "  deps          - Install dependencies"
	@echo "  run           - Build and run the application"
	@echo "  dev           - Run with hot reload"
	@echo "  fmt           - Format code"
	@echo "  lint          - Lint code"
	@echo "  security      - Run security checks"
	@echo "  docs          - Generate documentation"
	@echo "  docker-build  - Build Docker image"
	@echo "  docker-run    - Run Docker container"
	@echo "  help          - Show this help"

.PHONY: build clean test test-coverage benchmark build-linux deps run dev fmt lint security docs docker-build docker-run help