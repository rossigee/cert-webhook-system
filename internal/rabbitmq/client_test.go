package rabbitmq

import (
	"testing"
)

func TestNewClient_InvalidURL(t *testing.T) {
	_, err := NewClient("invalid://url")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}

func TestNewClient_EmptyURL(t *testing.T) {
	_, err := NewClient("")
	if err == nil {
		t.Error("Expected error for empty URL, got nil")
	}
}

func TestClient_CloseWithoutConnection(t *testing.T) {
	client := &Client{url: "amqp://test"}
	client.closed = true

	// Should not panic
	err := client.Close()
	if err != nil {
		t.Errorf("Expected no error closing unconnected client, got: %v", err)
	}
}

func TestClient_HealthCheck_NilConnection(t *testing.T) {
	client := &Client{url: "amqp://test"}

	err := client.HealthCheck()
	if err == nil {
		t.Error("Expected error for health check with nil connection")
	}
}
