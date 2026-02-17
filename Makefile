# Variables
BINARY_CONTROLLER = controller
BINARY_WEBHOOK = webhook
DOCKER_IMAGE = cert-webhook-system
DOCKER_TAG ?= latest
REGISTRY ?= harbor.golder.lan/library

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Go parameters
GOCMD = go
GOBUILD = $(GOCMD) build
GOCLEAN = $(GOCMD) clean
GOTEST = $(GOCMD) test
GOGET = $(GOCMD) get
GOMOD = $(GOCMD) mod

# Build flags
LDFLAGS = -s -w -X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE) -X main.gitCommit=$(GIT_COMMIT)

# Build targets
.PHONY: all build clean test deps controller webhook docker docker-push help

all: deps test build

## Build both binaries
build: controller webhook

## Build the controller binary
controller:
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -a -installsuffix cgo -ldflags="$(LDFLAGS)" -o $(BINARY_CONTROLLER) ./cmd/controller

## Build the webhook binary
webhook:
	CGO_ENABLED=0 GOOS=linux $(GOBUILD) -a -installsuffix cgo -ldflags="$(LDFLAGS)" -o $(BINARY_WEBHOOK) ./cmd/webhook

## Run tests
test:
	$(GOTEST) -v ./...

## Download dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

## Clean build artifacts
clean:
	$(GOCLEAN)
	rm -f $(BINARY_CONTROLLER) $(BINARY_WEBHOOK)

## Build Docker image
docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) .

## Build and push Docker image
docker-push: docker
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)
	docker push $(REGISTRY)/$(DOCKER_IMAGE):$(DOCKER_TAG)

## Run controller locally
run-controller: controller
	./$(BINARY_CONTROLLER) --log-level=debug

## Run webhook locally  
run-webhook: webhook
	./$(BINARY_WEBHOOK) --log-level=debug --port=8080

## Format code
fmt:
	$(GOCMD) fmt ./...

## Run linting
lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then echo "golangci-lint not found. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; fi
	@version=$$(golangci-lint version 2>&1 | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1); \
	if [ "$$(printf '%s\n' "$$version" "2.9.0" | sort -V | head -n1)" != "2.9.0" ]; then \
		echo "golangci-lint version $$version is too old. Please upgrade to >= 2.9.0"; \
		exit 1; \
	fi
	golangci-lint run

## Show help
help:
	@echo ''
	@echo 'Usage:'
	@echo '  make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-_0-9]+:/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")-1); \
			helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)