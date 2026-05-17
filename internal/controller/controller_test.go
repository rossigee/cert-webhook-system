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
		Clientset: clientset,
		Config:    &rest.Config{},
		Logger:    logr.Discard(),
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
		Clientset: clientset,
		Config:    &rest.Config{},
		Logger:    logr.Discard(),
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

	// Should fail because RabbitMQ is not configured
	err = ctrl.processCertificate(context.Background(), cert)
	if err == nil {
		t.Errorf("Expected error for ready cert without RabbitMQ, got nil")
	}
}

func TestProcessCertificate_DeduplicatesEvents(t *testing.T) {
	clientset := fake.NewClientset()

	ctrl, err := New(Config{
		Clientset:      clientset,
		Config:         &rest.Config{},
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

	// Simulate a prior successful processing by pre-loading the dedup key
	processKey := "default/test-cert:1"
	ctrl.processedCerts.Store(processKey, true)

	// Second call with same resourceVersion should be deduplicated
	err = ctrl.processCertificate(context.Background(), cert)
	if err != nil {
		t.Errorf("Expected no error on duplicate processing, got: %v", err)
	}
}
