package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rossigee/cert-webhook-system/internal/rabbitmq"
	"github.com/rossigee/cert-webhook-system/internal/webhook"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	version   = "dev"
	buildDate = "unknown"
	gitCommit = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "cert-webhook-handler",
	Short: "Certificate webhook HTTP handler",
	Long: `HTTP webhook handler that processes certificate renewal events and
publishes them to RabbitMQ for downstream consumption by fetch-k8s-cert agents.`,
	RunE: runWebhook,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("cert-webhook-handler version %s\n", version)
		fmt.Printf("  build date: %s\n", buildDate)
		fmt.Printf("  git commit: %s\n", gitCommit)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	rootCmd.PersistentFlags().String("kubeconfig", "", "Path to kubeconfig file")
	rootCmd.PersistentFlags().Int("port", 8080, "Port to listen on")
	rootCmd.PersistentFlags().String("rabbitmq-url", "", "RabbitMQ connection URL (required)")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")

	_ = viper.BindPFlag("kubeconfig", rootCmd.PersistentFlags().Lookup("kubeconfig"))
	_ = viper.BindPFlag("port", rootCmd.PersistentFlags().Lookup("port"))
	_ = viper.BindPFlag("rabbitmq-url", rootCmd.PersistentFlags().Lookup("rabbitmq-url"))
	_ = viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))

	viper.SetEnvPrefix("CERT_WEBHOOK")
	viper.AutomaticEnv()
}

func runWebhook(cmd *cobra.Command, args []string) error {
	opts := zap.Options{
		Development: viper.GetString("log-level") == "debug",
	}
	log.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	logger := log.Log.WithName("cert-webhook-handler")

	logger.Info("Starting Certificate Webhook Handler",
		"version", version,
		"build-date", buildDate,
		"git-commit", gitCommit,
		"port", viper.GetInt("port"),
		"log-level", viper.GetString("log-level"),
	)

	rmqURL := viper.GetString("rabbitmq-url")
	if rmqURL == "" {
		return fmt.Errorf("rabbitmq-url is required")
	}

	var config *rest.Config
	var err error

	if kubeconfigPath := viper.GetString("kubeconfig"); kubeconfigPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return fmt.Errorf("failed to build config from kubeconfig: %w", err)
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("failed to get in-cluster config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	rabbitmqClient, err := rabbitmq.NewClient(rmqURL)
	if err != nil {
		return fmt.Errorf("failed to create RabbitMQ client: %w", err)
	}
	defer func() { _ = rabbitmqClient.Close() }()

	handler, err := webhook.New(webhook.Config{
		Clientset:      clientset,
		Config:         config,
		RabbitMQClient: rabbitmqClient,
		Logger:         logger,
	})
	if err != nil {
		return fmt.Errorf("failed to create webhook handler: %w", err)
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", viper.GetInt("port")),
		Handler:      handler.Router(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("Received shutdown signal", "signal", sig)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error(err, "Server shutdown error")
		}
		cancel()
	}()

	logger.Info("Server starting", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	<-ctx.Done()
	logger.Info("Server stopped")
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
