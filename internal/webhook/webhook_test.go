package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/rossigee/cert-webhook-system/internal/event"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestNewHandler(t *testing.T) {
	clientset := fake.NewClientset()
	config := &rest.Config{}

	handler, err := New(Config{
		Clientset:      clientset,
		Config:         config,
		RabbitMQClient: nil,
		Logger:         logr.Discard(),
	})

	if err != nil {
		t.Errorf("Expected no error creating handler, got: %v", err)
	}

	if handler == nil {
		t.Error("Expected non-nil handler")
	}
}

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clientset := fake.NewClientset()
	handler, err := New(Config{
		Clientset:      clientset,
		Config:         &rest.Config{},
		RabbitMQClient: nil,
		Logger:         logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	router := handler.Router()

	req, _ := http.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse health check response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", response["status"])
	}

	if response["timestamp"] == nil {
		t.Error("Expected timestamp in health check response")
	}
}

func TestCertificateWebhookHandler_InvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clientset := fake.NewClientset()
	handler, err := New(Config{
		Clientset:      clientset,
		Config:         &rest.Config{},
		RabbitMQClient: nil,
		Logger:         logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	router := handler.Router()

	invalidJSON := `{"invalid": json}`
	req, _ := http.NewRequest("POST", "/webhook/certificate", bytes.NewBufferString(invalidJSON))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", w.Code)
	}
}

func TestCertificateWebhookHandler_MissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clientset := fake.NewClientset()
	handler, err := New(Config{
		Clientset:      clientset,
		Config:         &rest.Config{},
		RabbitMQClient: nil,
		Logger:         logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	router := handler.Router()

	payload := CertificateWebhookRequest{}
	payloadBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook/certificate", bytes.NewBuffer(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing fields, got %d", w.Code)
	}
}

func TestCertificateWebhookHandler_WebhookNotEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clientset := fake.NewClientset()
	handler, err := New(Config{
		Clientset:      clientset,
		Config:         &rest.Config{},
		RabbitMQClient: nil,
		Logger:         logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	router := handler.Router()

	payload := map[string]any{
		"metadata": map[string]any{
			"name":      "test-cert",
			"namespace": "default",
			"labels":    map[string]string{},
		},
		"spec": map[string]any{
			"secretName": "test-cert-tls",
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook/certificate", bytes.NewBuffer(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for ignored cert, got %d", w.Code)
	}

	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response["status"] != "ignored" {
		t.Errorf("Expected status 'ignored', got %v", response["status"])
	}
}

func TestCertificateWebhookHandler_NilRabbitMQ(t *testing.T) {
	gin.SetMode(gin.TestMode)

	clientset := fake.NewClientset()
	handler, err := New(Config{
		Clientset:      clientset,
		Config:         &rest.Config{},
		RabbitMQClient: nil,
		Logger:         logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	router := handler.Router()

	payload := map[string]any{
		"metadata": map[string]any{
			"name":      "test-cert",
			"namespace": "default",
			"labels": map[string]string{
				event.WebhookEnabledLabel: "true",
			},
		},
		"spec": map[string]any{
			"secretName": "test-cert-tls",
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook/certificate", bytes.NewBuffer(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503 for nil RabbitMQ, got %d", w.Code)
	}
}
