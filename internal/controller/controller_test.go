package controller

import (
	"context"
	"testing"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/go-logr/logr"
	"github.com/rossigee/cert-webhook-system/internal/event"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestNewController(t *testing.T) {
	clientset := fake.NewClientset()
	config := &rest.Config{}

	ctrl, err := New(Config{
		Clientset:      clientset,
		Config:         config,
		WebhookURL:     "http://test.example.com/webhook",
		RabbitMQClient: nil,
		Logger:         logr.Discard(),
	})

	if err != nil {
		t.Errorf("Expected no error creating controller, got: %v", err)
	}

	if ctrl == nil {
		t.Error("Expected non-nil controller")
	}
}

func TestProcessCertificate_WebhookNotEnabled(t *testing.T) {
	clientset := fake.NewClientset()

	ctrl, err := New(Config{
		Clientset:  clientset,
		Config:     &rest.Config{},
		WebhookURL: "http://test.example.com/webhook",
		Logger:     logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}

	cert := &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert",
			Namespace: "default",
			Labels:    map[string]string{},
		},
	}

	err = ctrl.processCertificate(context.Background(), cert)
	if err != nil {
		t.Errorf("Expected no error for cert without webhook label, got: %v", err)
	}
}

func TestProcessCertificate_NotReady(t *testing.T) {
	clientset := fake.NewClientset()

	ctrl, err := New(Config{
		Clientset:  clientset,
		Config:     &rest.Config{},
		WebhookURL: "http://test.example.com/webhook",
		Logger:     logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}

	cert := &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cert",
			Namespace: "default",
			Labels: map[string]string{
				event.WebhookEnabledLabel: "true",
			},
		},
		Status: certv1.CertificateStatus{
			Conditions: []certv1.CertificateCondition{
				{
					Type:   certv1.CertificateConditionReady,
					Status: cmmeta.ConditionFalse,
				},
			},
		},
	}

	err = ctrl.processCertificate(context.Background(), cert)
	if err != nil {
		t.Errorf("Expected no error for not-ready cert, got: %v", err)
	}
}

func TestProcessCertificate_ReadyNoRabbitMQ(t *testing.T) {
	clientset := fake.NewClientset()

	ctrl, err := New(Config{
		Clientset:      clientset,
		Config:         &rest.Config{},
		WebhookURL:     "http://test.example.com/webhook",
		RabbitMQClient: nil,
		Logger:         logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}

	cert := &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cert",
			Namespace:       "default",
			ResourceVersion: "1",
			Labels: map[string]string{
				event.WebhookEnabledLabel: "true",
			},
			Annotations: map[string]string{
				event.AnnotationPrefix + "target":        "docker-compose",
				event.AnnotationPrefix + "docker-engine": "docker.example.com",
			},
		},
		Spec: certv1.CertificateSpec{
			SecretName: "test-cert-tls",
		},
		Status: certv1.CertificateStatus{
			Conditions: []certv1.CertificateCondition{
				{
					Type:   certv1.CertificateConditionReady,
					Status: cmmeta.ConditionTrue,
					Reason: "Ready",
				},
			},
		},
	}

	// Should succeed without error (no RabbitMQ = skip publishing)
	err = ctrl.processCertificate(context.Background(), cert)
	if err != nil {
		t.Errorf("Expected no error for ready cert without RabbitMQ, got: %v", err)
	}
}

func TestProcessCertificate_DeduplicatesEvents(t *testing.T) {
	clientset := fake.NewClientset()

	ctrl, err := New(Config{
		Clientset:      clientset,
		Config:         &rest.Config{},
		WebhookURL:     "http://test.example.com/webhook",
		RabbitMQClient: nil,
		Logger:         logr.Discard(),
	})
	if err != nil {
		t.Fatalf("Failed to create controller: %v", err)
	}

	cert := &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cert",
			Namespace:       "default",
			ResourceVersion: "1",
			Labels: map[string]string{
				event.WebhookEnabledLabel: "true",
			},
		},
		Spec: certv1.CertificateSpec{
			SecretName: "test-cert-tls",
		},
		Status: certv1.CertificateStatus{
			Conditions: []certv1.CertificateCondition{
				{
					Type:   certv1.CertificateConditionReady,
					Status: cmmeta.ConditionTrue,
					Reason: "Ready",
				},
			},
		},
	}

	// Process twice with same resourceVersion
	_ = ctrl.processCertificate(context.Background(), cert)
	err = ctrl.processCertificate(context.Background(), cert)
	if err != nil {
		t.Errorf("Expected no error on duplicate processing, got: %v", err)
	}

	// Verify key was stored
	key := "default/test-cert:1"
	if _, ok := ctrl.processedCerts.Load(key); !ok {
		t.Error("Expected processedCerts to contain the certificate key")
	}
}
