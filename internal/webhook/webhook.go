package webhook

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rossigee/cert-webhook-system/internal/event"
	"github.com/rossigee/cert-webhook-system/internal/rabbitmq"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	webhooksReceivedTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "webhooks_received_total",
		Help: "Total number of webhook requests received",
	})

	rabbitmqPublishesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "rabbitmq_publishes_total",
		Help: "Total number of successful RabbitMQ publishes",
	})

	errorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "errors_total",
		Help: "Total number of errors encountered",
	})

	webhookRequestDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "webhook_request_duration_seconds",
		Help:    "Duration of webhook request processing in seconds",
		Buckets: prometheus.DefBuckets,
	})
)

func init() {
	prometheus.MustRegister(webhooksReceivedTotal)
	prometheus.MustRegister(rabbitmqPublishesTotal)
	prometheus.MustRegister(errorsTotal)
	prometheus.MustRegister(webhookRequestDuration)
}

// Config holds the configuration for the webhook handler
type Config struct {
	Clientset      kubernetes.Interface
	Config         *rest.Config
	RabbitMQClient *rabbitmq.Client
	Logger         logr.Logger
}

// Handler handles incoming webhook requests
type Handler struct {
	clientset      kubernetes.Interface
	config         *rest.Config
	rabbitmqClient *rabbitmq.Client
	logger         logr.Logger
	router         *gin.Engine
}

// CertificateWebhookRequest represents the incoming webhook payload
type CertificateWebhookRequest struct {
	Metadata struct {
		Name        string            `json:"name"`
		Namespace   string            `json:"namespace"`
		Labels      map[string]string `json:"labels"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		SecretName string `json:"secretName"`
	} `json:"spec"`
}

// New creates a new webhook handler
func New(config Config) (*Handler, error) {
	gin.SetMode(gin.ReleaseMode)

	handler := &Handler{
		clientset:      config.Clientset,
		config:         config.Config,
		rabbitmqClient: config.RabbitMQClient,
		logger:         config.Logger,
		router:         gin.New(),
	}

	handler.router.Use(gin.Recovery())
	handler.router.Use(handler.requestLogger())
	handler.setupRoutes()

	return handler, nil
}

// Router returns the HTTP router
func (h *Handler) Router() http.Handler {
	return h.router
}

// setupRoutes configures all HTTP routes
func (h *Handler) setupRoutes() {
	h.router.GET("/health", h.healthHandler)
	h.router.GET("/metrics", gin.WrapH(promhttp.Handler()))
	h.router.POST("/webhook/certificate", h.certificateWebhookHandler)
}

// requestLogger creates a Gin middleware for request logging
func (h *Handler) requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		c.Next()

		latency := time.Since(start)
		clientIP := c.ClientIP()
		method := c.Request.Method
		statusCode := c.Writer.Status()
		bodySize := c.Writer.Size()

		if raw != "" {
			path = path + "?" + raw
		}

		h.logger.Info("Request processed",
			"method", method,
			"path", path,
			"status", statusCode,
			"latency", latency,
			"ip", clientIP,
			"size", bodySize,
		)
	}
}

// healthHandler handles health check requests
func (h *Handler) healthHandler(c *gin.Context) {
	if h.rabbitmqClient != nil {
		if err := h.rabbitmqClient.HealthCheck(); err != nil {
			h.logger.Error(err, "RabbitMQ health check failed")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":   "unhealthy",
				"rabbitmq": "disconnected",
				"error":    err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"rabbitmq":  "connected",
		"timestamp": time.Now().Unix(),
	})
}

// certificateWebhookHandler handles certificate webhook requests
func (h *Handler) certificateWebhookHandler(c *gin.Context) {
	start := time.Now()
	webhooksReceivedTotal.Inc()

	var req CertificateWebhookRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		errorsTotal.Inc()
		h.logger.Error(err, "Failed to parse webhook request")
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid JSON payload",
			"details": err.Error(),
		})
		return
	}

	if req.Metadata.Name == "" || req.Metadata.Namespace == "" {
		errorsTotal.Inc()
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Missing required fields: name and namespace",
		})
		return
	}

	h.logger.Info("Received certificate webhook",
		"namespace", req.Metadata.Namespace,
		"name", req.Metadata.Name,
	)

	if req.Metadata.Labels[event.WebhookEnabledLabel] != "true" {
		h.logger.Info("Certificate does not have webhook enabled",
			"certificate", fmt.Sprintf("%s/%s", req.Metadata.Namespace, req.Metadata.Name))
		c.JSON(http.StatusOK, gin.H{
			"status": "ignored",
			"reason": "webhook not enabled",
		})
		return
	}

	if h.rabbitmqClient == nil {
		errorsTotal.Inc()
		h.logger.Error(nil, "RabbitMQ client not configured")
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "RabbitMQ not configured",
		})
		return
	}

	annotations := req.Metadata.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	message := event.NewMessage(req.Metadata.Name, req.Metadata.Namespace, req.Spec.SecretName, req.Metadata.Labels, annotations)
	exchange, routingKey := event.ExchangeAndRoutingKey(annotations)

	if err := h.rabbitmqClient.Publish(c.Request.Context(), exchange, routingKey, message); err != nil {
		errorsTotal.Inc()
		h.logger.Error(err, "Failed to publish to RabbitMQ",
			"certificate", fmt.Sprintf("%s/%s", req.Metadata.Namespace, req.Metadata.Name))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "RabbitMQ publish failed",
			"details": err.Error(),
		})
		return
	}

	rabbitmqPublishesTotal.Inc()
	webhookRequestDuration.Observe(time.Since(start).Seconds())

	h.logger.Info("Published certificate renewal event to RabbitMQ",
		"certificate", fmt.Sprintf("%s/%s", req.Metadata.Namespace, req.Metadata.Name),
		"exchange", exchange,
		"routing_key", routingKey,
		"docker_engine", annotations[event.AnnotationPrefix+"docker-engine"],
	)

	c.JSON(http.StatusOK, gin.H{
		"status":      "published",
		"certificate": req.Metadata.Name,
		"target":      annotations[event.AnnotationPrefix+"docker-engine"],
		"exchange":    exchange,
		"routing_key": routingKey,
		"containers":  message.ContainerNames,
	})
}
