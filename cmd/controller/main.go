package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rossigee/cert-webhook-system/internal/controller"
	"github.com/rossigee/cert-webhook-system/internal/rabbitmq"
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
	Use:   "cert-webhook-controller",
	Short: "Certificate webhook event controller",
	Long: `Watches Kubernetes Certificate resources and triggers webhook notifications
when certificates are renewed. Publishes events to RabbitMQ for downstream consumers.`,
	RunE: runController,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("cert-webhook-controller version %s\n", version)
		fmt.Printf("  build date: %s\n", buildDate)
		fmt.Printf("  git commit: %s\n", gitCommit)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)

	rootCmd.PersistentFlags().String("kubeconfig", "", "Path to kubeconfig file")
	rootCmd.PersistentFlags().String("webhook-url",
		"http://cert-webhook-handler.docker-stacks.svc.cluster.local/webhook/certificate",
		"Webhook URL to call")
	rootCmd.PersistentFlags().String("rabbitmq-url", "", "RabbitMQ connection URL")
	rootCmd.PersistentFlags().String("log-level", "info", "Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().Int("health-port", 9250, "Health check HTTP port")

	_ = viper.BindPFlag("kubeconfig", rootCmd.PersistentFlags().Lookup("kubeconfig"))
	_ = viper.BindPFlag("webhook-url", rootCmd.PersistentFlags().Lookup("webhook-url"))
	_ = viper.BindPFlag("rabbitmq-url", rootCmd.PersistentFlags().Lookup("rabbitmq-url"))
	_ = viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level"))
	_ = viper.BindPFlag("health-port", rootCmd.PersistentFlags().Lookup("health-port"))

	viper.SetEnvPrefix("CERT_WEBHOOK")
	viper.AutomaticEnv()
}

func runController(cmd *cobra.Command, args []string) error {
	opts := zap.Options{
		Development: viper.GetString("log-level") == "debug",
	}
	log.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	logger := log.Log.WithName("cert-webhook-controller")

	logger.Info("Starting Certificate Webhook Controller",
		"version", version,
		"build-date", buildDate,
		"git-commit", gitCommit,
		"webhook-url", viper.GetString("webhook-url"),
		"log-level", viper.GetString("log-level"),
	)

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

	var rabbitmqClient *rabbitmq.Client
	if rmqURL := viper.GetString("rabbitmq-url"); rmqURL != "" {
		rabbitmqClient, err = rabbitmq.NewClient(rmqURL)
		if err != nil {
			return fmt.Errorf("failed to create RabbitMQ client: %w", err)
		}
		defer func() { _ = rabbitmqClient.Close() }()
	}

	ctrl, err := controller.New(controller.Config{
		Clientset:      clientset,
		Config:         config,
		WebhookURL:     viper.GetString("webhook-url"),
		RabbitMQClient: rabbitmqClient,
		Logger:         logger,
		HealthPort:     viper.GetInt("health-port"),
	})
	if err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("Received shutdown signal", "signal", sig)
		cancel()
	}()

	return ctrl.Run(ctx)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
