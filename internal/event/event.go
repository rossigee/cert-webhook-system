package event

import (
	"strings"
	"time"
)

const (
	// WebhookEnabledLabel is the label key used to enable webhook notifications
	WebhookEnabledLabel = "cert-webhook.golder.tech/enabled"

	// AnnotationPrefix is the prefix for all webhook-related annotations
	AnnotationPrefix = "cert-webhook.golder.tech/"

	// DefaultExchange is the default RabbitMQ exchange name
	DefaultExchange = "certificate-events"

	// DefaultRoutingKey is the default RabbitMQ routing key
	DefaultRoutingKey = "certificate.renewed"
)

// Message represents a certificate renewal event message
type Message struct {
	Event             string         `json:"event"`
	Certificate       string         `json:"certificate"`
	Namespace         string         `json:"namespace"`
	SecretName        string         `json:"secret_name"`
	TargetType        string         `json:"target_type"`
	DockerEngine      string         `json:"docker_engine"`
	DockerComposePath string         `json:"docker_compose_path"`
	ContainerNames    []string       `json:"container_names"`
	Timestamp         int64          `json:"timestamp"`
	Trigger           string         `json:"trigger"`
	Metadata          map[string]any `json:"metadata"`
}

// NewMessage builds a certificate renewal event message from certificate metadata
func NewMessage(name, namespace, secretName string, labels, annotations map[string]string) Message {
	if annotations == nil {
		annotations = make(map[string]string)
	}

	return Message{
		Event:             "certificate.renewed",
		Certificate:       name,
		Namespace:         namespace,
		SecretName:        secretName,
		TargetType:        annotations[AnnotationPrefix+"target"],
		DockerEngine:      annotations[AnnotationPrefix+"docker-engine"],
		DockerComposePath: annotations[AnnotationPrefix+"docker-compose-path"],
		ContainerNames:    ParseContainerNames(annotations[AnnotationPrefix+"container-names"]),
		Timestamp:         time.Now().Unix(),
		Trigger:           "cert-manager-webhook",
		Metadata: map[string]any{
			"labels":      labels,
			"annotations": FilterAnnotations(annotations, AnnotationPrefix),
		},
	}
}

// ExchangeAndRoutingKey extracts the exchange and routing key from annotations,
// falling back to defaults
func ExchangeAndRoutingKey(annotations map[string]string) (string, string) {
	exchange := annotations[AnnotationPrefix+"rabbitmq-exchange"]
	if exchange == "" {
		exchange = DefaultExchange
	}

	routingKey := annotations[AnnotationPrefix+"rabbitmq-routing-key"]
	if routingKey == "" {
		routingKey = DefaultRoutingKey
	}

	return exchange, routingKey
}

// ParseContainerNames parses comma-separated container names
func ParseContainerNames(containerNamesStr string) []string {
	if containerNamesStr == "" {
		return []string{}
	}

	names := make([]string, 0)
	for name := range strings.SplitSeq(containerNamesStr, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

// FilterAnnotations returns only annotations with the specified prefix
func FilterAnnotations(annotations map[string]string, prefix string) map[string]string {
	filtered := make(map[string]string)
	for k, v := range annotations {
		if strings.HasPrefix(k, prefix) {
			filtered[k] = v
		}
	}
	return filtered
}
