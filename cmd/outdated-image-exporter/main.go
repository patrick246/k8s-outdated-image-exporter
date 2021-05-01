package main

import (
	"context"
	"flag"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/evaluation"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/exporter"
	pod_images "github.com/patrick246/k8s-outdated-image-exporter/pkg/pod-images"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/tags"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/version"
	"k8s.io/client-go/util/homedir"
	"log"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"
)

var inClusterConfig = flag.Bool("in-cluster", true, "Controls if the in-cluster connection configuration method should be used.")
var imageCheckInterval = flag.Duration("image-check-interval", time.Hour, "How often to check for new image versions. Configuring this to a lower interval will eat up your registry request quota faster.")
var registryCredentialsPath = flag.String("registry-credentials", path.Join(homedir.HomeDir(), ".docker", "config.json"), "Path to a file containing registry credentials. This is the same format as K8s imagePullSecret contents")
var listenAddr = flag.String("listen-addr", ":8080", "The address to listen on for metrics requests")

func main() {
	flag.Parse()

	podClient, err := pod_images.NewPodClient(pod_images.ConnectionConfig{
		InClusterConfig:        *inClusterConfig,
		InformerResyncInterval: 5 * time.Minute,
		ImageCheckInterval:     *imageCheckInterval,
	})
	if err != nil {
		log.Fatal(err)
	}

	authConfig, err := tags.ReadRegistryCredentialsFromFile(*registryCredentialsPath)
	if err != nil {
		log.Printf("No registry auth provided, looked up %s, got error: %v. Continuing without registry auth", *registryCredentialsPath, err)
	}
	tagLister, err := tags.NewTagLister(authConfig)
	if err != nil {
		log.Fatal(err)
	}

	versionChecker, err := version.NewChecker()
	if err != nil {
		log.Fatal(err)
	}

	evaluator, err := evaluation.NewEvaluator(podClient, tagLister, versionChecker)
	if err != nil {
		log.Fatal(err)
	}

	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGTERM)

	runCtx, cancel := context.WithCancel(context.Background())

	go func() {
		err = evaluator.Run(runCtx)
		if err != nil {
			log.Fatal(err)
		}
	}()

	go func() {
		err = exporter.RunServer(*listenAddr, runCtx)
		if err != nil {
			log.Fatal(err)
		}
	}()

	<-signals
	cancel()
}
