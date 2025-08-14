.PHONY: build clean router broker all

# Build all binaries
all: router broker

# Build the router (ext-proc service)
router:
	go build -o bin/mcp-router ./cmd/mcp-router

# Build the broker (simple HTTP server)
broker:
	go build -o bin/mcp-broker ./cmd/mcp-broker

# Build both binaries
build: all

# Clean build artifacts
clean:
	rm -rf bin/

# Run the router
run-router: router
	./bin/mcp-router

# Run the broker
run-broker: broker
	./bin/mcp-broker

# Download dependencies
deps:
	go mod download

# Update dependencies
update:
	go mod tidy
	go get -u ./...

# Lint

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: golangci-lint
golangci-lint:
	golangci-lint run ./...

.PHONY: lint
lint: fmt vet golangci-lint
