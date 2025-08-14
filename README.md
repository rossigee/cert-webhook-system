# Certificate Webhook System

A high-performance Go microservice system for handling certificate renewal events from cert-manager and distributing them via RabbitMQ to downstream consumers.

## Architecture

The system consists of two main components:

### 1. Certificate Event Controller (`controller`)
- Watches Kubernetes Certificate resources using the cert-manager API
- Detects when certificates are renewed (Ready=True condition)
- Publishes certificate renewal events to RabbitMQ
- Uses efficient Kubernetes informers and workqueues for high throughput

### 2. Certificate Webhook Handler (`webhook`)
- HTTP server that processes certificate webhook requests
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

| Variable | Description | Default |
|----------|-------------|---------|
| `CERT_WEBHOOK_KUBECONFIG` | Path to kubeconfig file | In-cluster config |
| `CERT_WEBHOOK_WEBHOOK_URL` | Webhook URL for controller | `http://cert-webhook-handler.docker-stacks.svc.cluster.local/webhook/certificate` |
| `CERT_WEBHOOK_RABBITMQ_URL` | RabbitMQ connection URL | Required for webhook handler |
| `CERT_WEBHOOK_PORT` | HTTP port for webhook handler | `8080` |
| `CERT_WEBHOOK_LOG_LEVEL` | Log level (debug/info/warn/error) | `info` |

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

## API Endpoints

### Webhook Handler

- `POST /webhook/certificate` - Process certificate renewal webhook
- `GET /health` - Health check endpoint
- `GET /metrics` - Basic metrics endpoint

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

The system is designed to be deployed in Kubernetes with the following components:

1. **ServiceAccount** with RBAC permissions for Certificate resources
2. **Deployment** for the controller (watches certificates)
3. **Deployment** for the webhook handler (HTTP server)
4. **Service** to expose the webhook handler
5. **ExternalSecret** for RabbitMQ credentials

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