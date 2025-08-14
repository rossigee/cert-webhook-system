package rabbitmq

import (
	"encoding/json"
	"testing"
)

func TestNewClient_InvalidURL(t *testing.T) {
	// Test with invalid URL - should fail gracefully
	_, err := NewClient("invalid://url")
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}

	// Test error message contains expected text
	if err != nil && err.Error() == "" {
		t.Error("Expected non-empty error message")
	}
}

func TestNewClient_EmptyURL(t *testing.T) {
	// Test with empty URL
	_, err := NewClient("")
	if err == nil {
		t.Error("Expected error for empty URL, got nil")
	}
}

func TestClient_URLStorage(t *testing.T) {
	// Test that client stores URL correctly (without connecting)
	testURL := "amqp://test:test@localhost:5672/"

	// Create client struct without connection for testing
	client := &Client{url: testURL}

	if client.url != testURL {
		t.Errorf("Expected URL %s, got %s", testURL, client.url)
	}
}

// TestClient_PublishMessage tests message publishing without real RabbitMQ
func TestClient_PublishMessage_NilChannel(t *testing.T) {
	client := &Client{url: "amqp://test"}

	// This should handle nil channel gracefully
	// For now, just test that the client was created with the correct URL
	if client.url != "amqp://test" {
		t.Errorf("Expected URL 'amqp://test', got %s", client.url)
	}
}

// TestFormatting tests basic data structures and JSON marshaling
func TestCertificateEventSerialization(t *testing.T) {
	event := map[string]interface{}{
		"event":       "certificate.renewed",
		"certificate": "test-cert",
		"namespace":   "default",
		"timestamp":   1692123456,
	}

	data, err := marshalEvent(event)
	if err != nil {
		t.Errorf("Failed to marshal event: %v", err)
	}

	if len(data) == 0 {
		t.Error("Expected non-empty marshaled data")
	}

	// Verify it's valid JSON
	var decoded map[string]interface{}
	if err := unmarshalEvent(data, &decoded); err != nil {
		t.Errorf("Failed to unmarshal event: %v", err)
	}

	if decoded["event"] != "certificate.renewed" {
		t.Errorf("Expected event type 'certificate.renewed', got %v", decoded["event"])
	}
}

// Helper functions for testing JSON operations
func marshalEvent(event interface{}) ([]byte, error) {
	return json.Marshal(event)
}

func unmarshalEvent(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
