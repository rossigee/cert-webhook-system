# Certificate Webhook System

A high-performance Go microservice for handling certificate renewal events from cert-manager and publishing them to RabbitMQ for downstream consumers.

## What This Is

This repository provides **event producers** — two independent ingress paths that detect certificate renewal events and normalize them into a shared RabbitMQ event stream.

**It does NOT include downstream consumers.** The services that read from RabbitMQ and act on certificate events (e.g., restarting Docker containers, reloading reverse proxies) live in other repositories.

## Architecture

```
┌──────────────────┐     ┌──────────────────┐
│   cert-manager   │     │   External HTTP  │
│   (Ready=True)   │     │   POST request   │
└────────┬─────────┘     └────────┬─────────┘
         │                        │
    ┌────▼────┐              ┌────▼────┐
    │Controller│              │ Webhook │
    │ (watcher)│              │ (server)│
    └────┬─────┘              └────┬────┘
         │                        │
         └────────┬───────────────┘
                  │
            ┌─────▼──────┐
            │  RabbitMQ  │
            │  Exchange  │
            └─────┬──────┘
                  │
         ┌────────▼────────┐
         │   Downstream    │
         │   Consumers     │
         │  (other repos)  │
         └─────────────────┘
```

Both binaries produce the **same event format** into the **same RabbitMQ exchange**. They do not call each other.

### Binary Comparison

| Binary | Role | Input | Output | When to Use |
|--------|------|-------|--------|-------------|
| `controller` | Kubernetes-native producer | Watches Certificate resources via K8s informers | Publishes to RabbitMQ | You want automatic detection when cert-manager renews a certificate |
| `webhook` | HTTP ingress producer | Accepts `POST /webhook/certificate` | Publishes to RabbitMQ | You want external systems (or cert-manager webhooks) to push events via HTTP |

### 1. Certificate Event Controller (`./cmd/controller/`)
- Watches Kubernetes Certificate resources using the cert-manager API
- Detects when certificates reach Ready=True state
- Publishes certificate renewal events to RabbitMQ
- Uses efficient Kubernetes informers and workqueues for high throughput

### 2. Certificate Webhook Handler (`./cmd/webhook/`)
- HTTP server that accepts certificate webhook requests
- Validates certificate metadata and webhook configuration
- Publishes events to RabbitMQ with proper routing
- Provides health checks and basic metrics endpoints

## Features

- **High Performance**: Go-based microservices with minimal memory footprint (~10MB)
- **Kubernetes Native**: Uses cert-manager APIs and Kubernetes informers
- **Event-Driven**: RabbitMQ-based event distribution with proper exchange/routing
- **Container Ready**: Scratch-based Docker images for security and size
- **Observability**: Structured logging, health checks, and metrics endpoints
- **Production Ready**: Graceful shutdown, error handling, and reconnection logic

## Configuration

### Environment Variables

#### Controller (`cmd/controller/`)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CERT_WEBHOOK_KUBECONFIG` | Path to kubeconfig file | In-cluster config | No |
| `CERT_WEBHOOK_RABBITMQ_URL` | RabbitMQ connection URL | — | **Yes** |
| `CERT_WEBHOOK_LOG_LEVEL` | Log level (debug/info/warn/error) | `info` | No |

#### Webhook Handler (`cmd/webhook/`)

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `CERT_WEBHOOK_KUBECONFIG` | Path to kubeconfig file | In-cluster config | No |
| `CERT_WEBHOOK_RABBITMQ_URL` | RabbitMQ connection URL | — | **Yes** |
| `CERT_WEBHOOK_PORT` | HTTP port | `8080` | No |
| `CERT_WEBHOOK_LOG_LEVEL` | Log level (debug/info/warn/error) | `info` | No |

### Certificate Labeling

To enable webhook notifications for a certificate, add the following label:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example-tls
  namespace: default
  labels:
    cert-webhook.golder.tech/enabled: "true"
    cert-webhook.golder.tech/target: "docker-compose"
  annotations:
    cert-webhook.golder.tech/docker-engine: "docker.example.com"
    cert-webhook.golder.tech/docker-compose-path: "/docker/stacks/example"
    cert-webhook.golder.tech/container-names: "nginx,api-server"
    cert-webhook.golder.tech/rabbitmq-exchange: "certificate-events"
    cert-webhook.golder.tech/rabbitmq-routing-key: "certificate.renewed"
spec:
  secretName: example-tls
  # ... rest of certificate spec
```

## What is NOT in This Repository

- **RabbitMQ consumers** — There is no code here that reads from RabbitMQ. Downstream consumers that react to certificate events (e.g., restarting containers, reloading proxies) are separate services maintained elsewhere.
- **cert-manager itself** — This system assumes cert-manager is already installed in your cluster.
- **RabbitMQ server** — You must provide your own RabbitMQ instance or cluster.

## API Endpoints

### Webhook Handler

- `POST /webhook/certificate` - Accept certificate renewal event via HTTP
- `GET /health` - Health check endpoint (tests RabbitMQ connectivity)
- `GET /metrics` - Prometheus metrics endpoint

## Building

```bash
# Build both binaries
make build

# Build Docker image
make docker

# Build and push to registry
make docker-push REGISTRY=harbor.golder.lan/library DOCKER_TAG=v1.0.0

# Run tests
make test

# Local development
make run-controller  # Run controller locally
make run-webhook     # Run webhook handler locally
```

## Deployment

### Kubernetes Manifests

The manifest deploys both ingress components independently:

1. **ServiceAccount** with RBAC permissions for Certificate resources
2. **Deployment** for the controller (K8s watcher → RabbitMQ)
3. **Deployment** for the webhook handler (HTTP server → RabbitMQ)
4. **Service** to expose the webhook handler HTTP endpoints
5. **ExternalSecret** for RabbitMQ credentials

You can deploy both, or only the component(s) you need. They operate independently.

### RabbitMQ Integration

The system publishes certificate renewal events to RabbitMQ with the following structure:

```json
{
  "event": "certificate.renewed",
  "certificate": "example-tls",
  "namespace": "default",
  "secret_name": "example-tls",
  "target_type": "docker-compose",
  "docker_engine": "docker.example.com",
  "docker_compose_path": "/docker/stacks/example",
  "container_names": ["nginx", "api-server"],
  "timestamp": 1692123456,
  "trigger": "cert-manager-webhook",
  "metadata": {
    "labels": {...},
    "annotations": {...}
  }
}
```

## Monitoring

### Health Checks

The webhook handler provides a `/health` endpoint that:
- Tests RabbitMQ connectivity
- Returns HTTP 200 (healthy) or 503 (unhealthy)
- Includes timestamp and connection status

### Logging

Both components use structured logging with the following fields:
- `timestamp` - RFC3339 timestamp
- `level` - Log level (debug/info/warn/error)
- `msg` - Human readable message
- Context-specific fields (certificate names, namespaces, etc.)

### Metrics

Basic metrics are available at `/metrics` endpoint:
- `webhooks_received_total` - Total webhook requests processed
- `rabbitmq_publishes_total` - Total messages published to RabbitMQ
- `errors_total` - Total errors encountered

## Development

### Prerequisites

- Go 1.25+
- Docker
- kubectl
- Access to a Kubernetes cluster with cert-manager

### Local Development

1. Clone the repository:
```bash
git clone https://github.com/rossigee/cert-webhook-system
cd cert-webhook-system
```

2. Install dependencies:
```bash
make deps
```

3. Run tests:
```bash
make test
```

4. Build binaries:
```bash
make build
```

5. Run locally (requires kubeconfig and RabbitMQ):
```bash
export CERT_WEBHOOK_RABBITMQ_URL="amqp://user:pass@localhost:5672/"
make run-controller &
make run-webhook
```

## License

This project is part of the Golder infrastructure automation system.