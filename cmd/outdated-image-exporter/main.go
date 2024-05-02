package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/client-go/util/homedir"

	"github.com/patrick246/k8s-outdated-image-exporter/pkg/clients/docker"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/clients/k8s"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/evaluation"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/exporter"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/tags"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/version"
)

var inClusterConfig = flag.Bool("in-cluster", true, "Controls if the in-cluster connection configuration method should be used.")
var imageCheckInterval = flag.Duration("image-check-interval", time.Hour, "How often to check for new image versions. Configuring this to a lower interval will eat up your registry request quota faster.")
var registryCredentialsPath = flag.String("registry-credentials", path.Join(homedir.HomeDir(), ".docker", "config.json"), "Path to a file containing registry credentials. This is the same format as K8s imagePullSecret contents")
var listenAddr = flag.String("listen-addr", ":8080", "The address to listen on for metrics requests")
var containerProvider = flag.String("container", "kubernetes", "Container technology used: [kubernetes, docker]")
var logLevel = flag.String("log-level", "info", "Log level: [debug, info, warning, error]")

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v", err)

		os.Exit(1)
	}
}

func run() error {
	flag.Parse()

	var level slog.Level
	err := level.UnmarshalText([]byte(*logLevel))
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		AddSource: true,
		Level:     level,
	}))

	var client evaluation.ContainerClient

	switch *containerProvider {
	case "kubernetes":
		k8sClient, err := k8s.NewContainerClient(k8s.ConnectionConfig{
			InClusterConfig:        *inClusterConfig,
			InformerResyncInterval: 5 * time.Minute,
			ImageCheckInterval:     *imageCheckInterval,
		}, logger)
		if err != nil {
			return err
		}

		client = k8sClient
	case "docker":
		dockerClient, err := docker.NewDockerClient(logger)
		if err != nil {
			return err
		}

		client = dockerClient
	default:
		return fmt.Errorf("unsupported provider %q", *containerProvider)
	}

	authConfig, err := tags.ReadRegistryCredentialsFromFile(*registryCredentialsPath)
	if err != nil {
		logger.Warn("no registry auth provided. continuing without registry auth", "path", *registryCredentialsPath, "error", err)
	}

	tagLister, err := tags.NewTagLister(authConfig)
	if err != nil {
		return err
	}

	versionChecker, err := version.NewChecker()
	if err != nil {
		return err
	}

	evaluator, err := evaluation.NewEvaluator(tagLister, versionChecker, client, logger)
	if err != nil {
		return err
	}

	metricsCollector := exporter.NewCollector(evaluator)

	err = prometheus.Register(metricsCollector)
	if err != nil {
		return err
	}

	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGTERM)

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		err = evaluator.Run(runCtx)
		if err != nil {
			logger.Error("failed to run evaluator", "error", err)

			os.Exit(1)
		}
	}()

	shutdownFunc, err := exporter.RunServer(*listenAddr)
	if err != nil {
		return err
	}

	<-signals

	cancel()

	err = shutdownFunc()
	if err != nil {
		return err
	}

	return nil
}
