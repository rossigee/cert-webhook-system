# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a high-performance Go microservice system for handling certificate renewal events from cert-manager and distributing them via RabbitMQ to downstream consumers. The system consists of two main components that work together to provide event-driven certificate deployment automation.

## Architecture

The system follows a microservices pattern with two distinct components:

### 1. Certificate Event Controller (`./cmd/controller/`)
- **Purpose**: Kubernetes controller that watches Certificate resources using cert-manager APIs
- **Technology**: Uses Kubernetes informers and workqueues for efficient event processing
- **Trigger**: Detects when certificates reach Ready=True state
- **Output**: Publishes renewal events to RabbitMQ and/or calls webhook endpoint
- **Key Files**: 
  - `cmd/controller/main.go` - Application entry point with CLI setup
  - `internal/controller/controller.go` - Core controller logic with Kubernetes informers

### 2. Certificate Webhook Handler (`./cmd/webhook/`)
- **Purpose**: HTTP server that processes certificate webhook requests
- **Technology**: Gin-based HTTP server with structured logging
- **Input**: HTTP POST requests with certificate metadata
- **Output**: Publishes events to RabbitMQ for downstream consumption
- **Key Files**:
  - `cmd/webhook/main.go` - HTTP server setup and configuration
  - `internal/webhook/webhook.go` - HTTP handlers and request processing

### Supporting Components
- **RabbitMQ Client** (`internal/rabbitmq/client.go`): Robust RabbitMQ integration with connection management, auto-reconnection, and health checks
- **Event Message Format**: Structured JSON with certificate metadata, deployment targets, and container information

## Certificate Configuration

Certificates must be labeled and annotated to enable webhook processing:

**Required Label**:
- `cert-webhook.golder.tech/enabled: "true"`

**Key Annotations** (all prefixed with `cert-webhook.golder.tech/`):
- `target` - Deployment target type (e.g., "docker-compose")
- `docker-engine` - Target Docker host
- `docker-compose-path` - Path to docker-compose files
- `container-names` - Comma-separated list of containers to restart
- `rabbitmq-exchange` - Custom exchange (defaults to "certificate-events")
- `rabbitmq-routing-key` - Custom routing key (defaults to "certificate.renewed")

## Development Commands

### Building
```bash
# Build both binaries
make build

# Build individual components
make controller
make webhook

# Build Docker image
make docker

# Build and push to registry
make docker-push REGISTRY=harbor.golder.lan/library DOCKER_TAG=v1.0.0
```

### Testing and Quality
```bash
# Run tests
make test

# Download dependencies
make deps

# Format code
make fmt

# Run linting (requires golangci-lint)
make lint
```

### Local Development
```bash
# Run controller locally (requires kubeconfig and optional RabbitMQ)
make run-controller

# Run webhook handler locally (requires RabbitMQ URL)
export CERT_WEBHOOK_RABBITMQ_URL="amqp://user:pass@localhost:5672/"
make run-webhook
```

### Cleaning
```bash
# Clean build artifacts
make clean
```

## Environment Configuration

Both components use environment variables prefixed with `CERT_WEBHOOK_`:

### Controller Environment Variables
- `CERT_WEBHOOK_KUBECONFIG` - Path to kubeconfig (optional, uses in-cluster config)
- `CERT_WEBHOOK_WEBHOOK_URL` - Webhook endpoint URL 
- `CERT_WEBHOOK_RABBITMQ_URL` - RabbitMQ connection string (optional)
- `CERT_WEBHOOK_LOG_LEVEL` - Logging level (debug/info/warn/error)

### Webhook Handler Environment Variables
- `CERT_WEBHOOK_KUBECONFIG` - Path to kubeconfig (optional)
- `CERT_WEBHOOK_PORT` - HTTP port (default: 8080)
- `CERT_WEBHOOK_RABBITMQ_URL` - RabbitMQ connection string (required)
- `CERT_WEBHOOK_LOG_LEVEL` - Logging level

## Key Design Patterns

### Kubernetes Controller Pattern
- Uses client-go informers for efficient Kubernetes resource watching
- Implements workqueue pattern for reliable event processing
- Includes duplicate event prevention using certificate generation tracking
- Follows controller-runtime patterns without the full framework

### Event-Driven Architecture
- Publishes structured JSON events to RabbitMQ topic exchanges
- Uses routing keys for message filtering by consumers
- Supports both direct RabbitMQ publishing and HTTP webhook patterns
- Implements graceful degradation when RabbitMQ is unavailable

### Production Readiness Features
- Scratch-based Docker images for minimal attack surface
- Comprehensive health checks with RabbitMQ connectivity testing
- Structured logging with contextual information
- Graceful shutdown handling for both HTTP server and controller
- Automatic RabbitMQ reconnection with exponential backoff

## File Structure Patterns

- `cmd/` - Application entry points with CLI configuration
- `internal/` - Private application packages not meant for import
- `deploy/kubernetes/` - Kubernetes manifests with RBAC and deployments
- Root level Makefile, Dockerfile, and Go modules follow standard patterns

## Testing

Run tests with `make test`. The codebase uses Go's built-in testing framework. When adding new features:
- Add unit tests for business logic in controller and webhook packages
- Test RabbitMQ integration scenarios including connection failures
- Verify Kubernetes client interactions using fake clients where appropriate

## Security Considerations

- Both services run as non-root user (65534)
- Read-only root filesystems with no privilege escalation
- RBAC permissions limited to reading Certificate and Secret resources
- RabbitMQ credentials managed via Kubernetes secrets
- No sensitive data logged or exposed in HTTP responses

## Deployment

Deploy using `kubectl apply -f deploy/kubernetes/cert-webhook-system.yaml`. The manifest includes:
- ServiceAccount with minimal RBAC permissions
- Separate deployments for controller and webhook handler
- Service for webhook handler HTTP endpoints  
- ServiceMonitor for Prometheus metrics collection
- Resource limits and security contexts for both components