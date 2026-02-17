package event

import (
	"testing"
)

func TestParseContainerNames(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single container",
			input:    "nginx",
			expected: []string{"nginx"},
		},
		{
			name:     "multiple containers",
			input:    "nginx,api,worker",
			expected: []string{"nginx", "api", "worker"},
		},
		{
			name:     "with whitespace",
			input:    " nginx , api , worker ",
			expected: []string{"nginx", "api", "worker"},
		},
		{
			name:     "trailing comma",
			input:    "nginx,api,",
			expected: []string{"nginx", "api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseContainerNames(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d names, got %d: %v", len(tt.expected), len(result), result)
			}
			for i, name := range result {
				if name != tt.expected[i] {
					t.Errorf("expected name[%d] = %q, got %q", i, tt.expected[i], name)
				}
			}
		})
	}
}

func TestFilterAnnotations(t *testing.T) {
	annotations := map[string]string{
		"cert-webhook.golder.tech/docker-engine": "docker.example.com",
		"cert-webhook.golder.tech/target":        "docker-compose",
		"app.kubernetes.io/name":                 "test-app",
		"other-annotation":                       "value",
	}

	result := FilterAnnotations(annotations, AnnotationPrefix)

	if len(result) != 2 {
		t.Fatalf("expected 2 filtered annotations, got %d", len(result))
	}

	if result["cert-webhook.golder.tech/docker-engine"] != "docker.example.com" {
		t.Error("expected docker-engine annotation to be present")
	}

	if result["cert-webhook.golder.tech/target"] != "docker-compose" {
		t.Error("expected target annotation to be present")
	}

	if _, ok := result["app.kubernetes.io/name"]; ok {
		t.Error("non-prefixed annotation should not be included")
	}
}

func TestFilterAnnotations_NilMap(t *testing.T) {
	result := FilterAnnotations(nil, AnnotationPrefix)
	if len(result) != 0 {
		t.Errorf("expected empty map for nil input, got %d entries", len(result))
	}
}

func TestNewMessage(t *testing.T) {
	labels := map[string]string{
		"cert-webhook.golder.tech/enabled": "true",
	}
	annotations := map[string]string{
		"cert-webhook.golder.tech/target":              "docker-compose",
		"cert-webhook.golder.tech/docker-engine":       "docker.example.com",
		"cert-webhook.golder.tech/docker-compose-path": "/docker/stacks/app",
		"cert-webhook.golder.tech/container-names":     "nginx,api",
	}

	msg := NewMessage("test-cert", "default", "test-cert-tls", labels, annotations)

	if msg.Event != "certificate.renewed" {
		t.Errorf("expected event 'certificate.renewed', got %q", msg.Event)
	}
	if msg.Certificate != "test-cert" {
		t.Errorf("expected certificate 'test-cert', got %q", msg.Certificate)
	}
	if msg.Namespace != "default" {
		t.Errorf("expected namespace 'default', got %q", msg.Namespace)
	}
	if msg.SecretName != "test-cert-tls" {
		t.Errorf("expected secret_name 'test-cert-tls', got %q", msg.SecretName)
	}
	if msg.TargetType != "docker-compose" {
		t.Errorf("expected target_type 'docker-compose', got %q", msg.TargetType)
	}
	if msg.DockerEngine != "docker.example.com" {
		t.Errorf("expected docker_engine 'docker.example.com', got %q", msg.DockerEngine)
	}
	if msg.DockerComposePath != "/docker/stacks/app" {
		t.Errorf("expected docker_compose_path '/docker/stacks/app', got %q", msg.DockerComposePath)
	}
	if len(msg.ContainerNames) != 2 || msg.ContainerNames[0] != "nginx" || msg.ContainerNames[1] != "api" {
		t.Errorf("expected container_names [nginx, api], got %v", msg.ContainerNames)
	}
	if msg.Trigger != "cert-manager-webhook" {
		t.Errorf("expected trigger 'cert-manager-webhook', got %q", msg.Trigger)
	}
	if msg.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewMessage_NilAnnotations(t *testing.T) {
	msg := NewMessage("test-cert", "default", "test-cert-tls", nil, nil)

	if msg.Certificate != "test-cert" {
		t.Errorf("expected certificate 'test-cert', got %q", msg.Certificate)
	}
	if msg.TargetType != "" {
		t.Errorf("expected empty target_type, got %q", msg.TargetType)
	}
	if len(msg.ContainerNames) != 0 {
		t.Errorf("expected empty container_names, got %v", msg.ContainerNames)
	}
}

func TestExchangeAndRoutingKey(t *testing.T) {
	tests := []struct {
		name            string
		annotations     map[string]string
		expectedExch    string
		expectedRouting string
	}{
		{
			name:            "defaults",
			annotations:     map[string]string{},
			expectedExch:    DefaultExchange,
			expectedRouting: DefaultRoutingKey,
		},
		{
			name: "custom values",
			annotations: map[string]string{
				"cert-webhook.golder.tech/rabbitmq-exchange":    "custom-exchange",
				"cert-webhook.golder.tech/rabbitmq-routing-key": "custom.key",
			},
			expectedExch:    "custom-exchange",
			expectedRouting: "custom.key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exch, rk := ExchangeAndRoutingKey(tt.annotations)
			if exch != tt.expectedExch {
				t.Errorf("expected exchange %q, got %q", tt.expectedExch, exch)
			}
			if rk != tt.expectedRouting {
				t.Errorf("expected routing key %q, got %q", tt.expectedRouting, rk)
			}
		})
	}
}
