package main

import (
	"context"
	"flag"
	pod_images "github.com/patrick246/k8s-outdated-image-exporter/pkg/pod-images"
	"k8s.io/client-go/util/homedir"
	"log"
	"os"
	"path"
	"time"
)

var inClusterConfig = flag.Bool("in-cluster", true, "Controls if the in-cluster connection configuration method should be used.")
var kubeconfigFlag = flag.String("kubeconfig", path.Join(homedir.HomeDir(), ".kube", "config"), "Kubeconfig file, if in-cluster connection is not used")
var imageCheckInterval = flag.Duration("image-check-interval", time.Hour, "How often to check for new image versions. Configuring this to a lower interval will eat up your registry request quota faster.")

func main() {
	flag.Parse()

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = *kubeconfigFlag
	}

	podClient, err := pod_images.NewPodClient(pod_images.ConnectionConfig{
		InClusterConfig:        *inClusterConfig,
		KubeconfigPath:         kubeconfig,
		InformerResyncInterval: 5 * time.Minute,
		ImageCheckInterval:     *imageCheckInterval,
	})
	if err != nil {
		log.Fatal(err)
	}

	err = podClient.Listen(context.Background(), func(images pod_images.PodImages) error {
		log.Printf("Here would be a check for the images %q", images.Images)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}
