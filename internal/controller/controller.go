package controller

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certclient "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned"
	certinformers "github.com/cert-manager/cert-manager/pkg/client/informers/externalversions"
	certlisters "github.com/cert-manager/cert-manager/pkg/client/listers/certmanager/v1"
	"github.com/go-logr/logr"
	"github.com/rossigee/cert-webhook-system/internal/event"
	"github.com/rossigee/cert-webhook-system/internal/rabbitmq"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Config holds the configuration for the certificate controller
type Config struct {
	Clientset      kubernetes.Interface
	Config         *rest.Config
	WebhookURL     string
	RabbitMQClient *rabbitmq.Client
	Logger         logr.Logger
	HealthPort     int
}

// Controller watches Certificate resources and triggers webhooks
type Controller struct {
	clientset          kubernetes.Interface
	certClient         certclient.Interface
	informerFactory    certinformers.SharedInformerFactory
	certificateLister  certlisters.CertificateLister
	certificatesSynced cache.InformerSynced
	workqueue          workqueue.TypedRateLimitingInterface[string]
	webhookURL         string
	rabbitmqClient     *rabbitmq.Client
	logger             logr.Logger
	processedCerts     sync.Map
	cacheSynced        atomic.Bool
	healthPort         int
}

// New creates a new certificate controller
func New(config Config) (*Controller, error) {
	certClient, err := certclient.NewForConfig(config.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create cert-manager client: %w", err)
	}

	certInformerFactory := certinformers.NewSharedInformerFactory(certClient, time.Second*30)
	certificateInformer := certInformerFactory.Certmanager().V1().Certificates()

	healthPort := config.HealthPort
	if healthPort == 0 {
		healthPort = 9250
	}

	controller := &Controller{
		clientset:          config.Clientset,
		certClient:         certClient,
		informerFactory:    certInformerFactory,
		certificateLister:  certificateInformer.Lister(),
		certificatesSynced: certificateInformer.Informer().HasSynced,
		workqueue:          workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[string]()),
		webhookURL:         config.WebhookURL,
		rabbitmqClient:     config.RabbitMQClient,
		logger:             config.Logger,
		healthPort:         healthPort,
	}

	config.Logger.Info("Setting up event handlers")

	_, _ = certificateInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueCertificate,
		UpdateFunc: func(old, new any) {
			controller.enqueueCertificate(new)
		},
	})

	return controller, nil
}

// Run starts the controller
func (c *Controller) Run(ctx context.Context) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	c.logger.Info("Starting certificate webhook controller")

	// Start health server
	go c.startHealthServer(ctx)

	// Start informer factory
	c.informerFactory.Start(ctx.Done())

	c.logger.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(ctx.Done(), c.certificatesSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	c.cacheSynced.Store(true)

	c.logger.Info("Starting worker")
	go wait.UntilWithContext(ctx, c.runWorker, time.Second)

	c.logger.Info("Controller started")
	<-ctx.Done()
	c.logger.Info("Shutting down controller")

	return nil
}

// startHealthServer runs an HTTP health server for Kubernetes probes
func (c *Controller) startHealthServer(ctx context.Context) {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !c.cacheSynced.Load() {
			http.Error(w, "cache not synced", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "ok")
	})

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", c.healthPort),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	c.logger.Info("Starting health server", "port", c.healthPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		c.logger.Error(err, "Health server error")
	}
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the workqueue
func (c *Controller) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler
func (c *Controller) processNextWorkItem(ctx context.Context) bool {
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}

	err := func(obj string) error {
		defer c.workqueue.Done(obj)
		key := obj

		if err := c.syncHandler(ctx, key); err != nil {
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}

		c.workqueue.Forget(obj)
		c.logger.Info("Successfully synced certificate", "key", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two
func (c *Controller) syncHandler(ctx context.Context, key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	certificate, err := c.certificateLister.Certificates(namespace).Get(name)
	if err != nil {
		return fmt.Errorf("error getting certificate %s/%s: %w", namespace, name, err)
	}

	return c.processCertificate(ctx, certificate)
}

// processCertificate processes a certificate and triggers webhook if needed
func (c *Controller) processCertificate(ctx context.Context, cert *certv1.Certificate) error {
	if cert.Labels[event.WebhookEnabledLabel] != "true" {
		return nil
	}

	ready := false
	for _, condition := range cert.Status.Conditions {
		if condition.Type == certv1.CertificateConditionReady &&
			condition.Status == "True" &&
			condition.Reason == "Ready" {
			ready = true
			break
		}
	}

	if !ready {
		return nil
	}

	// Use namespace/name:resourceVersion as the dedup key
	processKey := fmt.Sprintf("%s/%s:%s", cert.Namespace, cert.Name, cert.ResourceVersion)

	if _, loaded := c.processedCerts.LoadOrStore(processKey, true); loaded {
		return nil
	}

	c.logger.Info("Certificate is ready, triggering webhook",
		"namespace", cert.Namespace,
		"name", cert.Name,
		"resourceVersion", cert.ResourceVersion,
	)

	if err := c.triggerWebhook(ctx, cert); err != nil {
		c.processedCerts.Delete(processKey)
		return fmt.Errorf("failed to trigger webhook for certificate %s/%s: %w",
			cert.Namespace, cert.Name, err)
	}

	return nil
}

// triggerWebhook sends a webhook notification for the certificate
func (c *Controller) triggerWebhook(ctx context.Context, cert *certv1.Certificate) error {
	if c.rabbitmqClient != nil {
		return c.publishToRabbitMQ(ctx, cert)
	}

	c.logger.Info("No RabbitMQ client available, skipping webhook",
		"certificate", fmt.Sprintf("%s/%s", cert.Namespace, cert.Name))

	return nil
}

// publishToRabbitMQ publishes certificate renewal event to RabbitMQ
func (c *Controller) publishToRabbitMQ(ctx context.Context, cert *certv1.Certificate) error {
	annotations := cert.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	message := event.NewMessage(cert.Name, cert.Namespace, cert.Spec.SecretName, cert.Labels, annotations)
	exchange, routingKey := event.ExchangeAndRoutingKey(annotations)

	if err := c.rabbitmqClient.Publish(ctx, exchange, routingKey, message); err != nil {
		return fmt.Errorf("failed to publish to RabbitMQ: %w", err)
	}

	c.logger.Info("Published certificate renewal event to RabbitMQ",
		"certificate", fmt.Sprintf("%s/%s", cert.Namespace, cert.Name),
		"exchange", exchange,
		"routing_key", routingKey,
	)

	return nil
}

// enqueueCertificate takes a Certificate resource and converts it into a namespace/name
// string which is then put onto the work queue
func (c *Controller) enqueueCertificate(obj any) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}
