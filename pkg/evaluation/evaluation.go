package evaluation

import (
	"context"
	"fmt"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/exporter"
	pod_images "github.com/patrick246/k8s-outdated-image-exporter/pkg/pod-images"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/tags"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/version"
	"github.com/prometheus/client_golang/prometheus"
	"log"
)

type Evaluator struct {
	podClient      *pod_images.PodClient
	tagLister      *tags.TagLister
	versionChecker *version.Checker

	podContainers map[string][]string
}

func NewEvaluator(podClient *pod_images.PodClient, tagLister *tags.TagLister, versionChecker *version.Checker) (*Evaluator, error) {
	return &Evaluator{
		podClient:      podClient,
		tagLister:      tagLister,
		versionChecker: versionChecker,
		podContainers:  map[string][]string{},
	}, nil
}

func (e *Evaluator) Run(ctx context.Context) error {
	return e.podClient.Listen(ctx, func(pod pod_images.PodImages, removed bool) error {
		if removed {
			for _, container := range e.podContainers[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)] {
				exporter.OutdatedMetric.Delete(prometheus.Labels{
					"namespace": pod.Namespace,
					"pod":       pod.Name,
					"container": container,
					"type":      "major",
				})
				exporter.OutdatedMetric.Delete(prometheus.Labels{
					"namespace": pod.Namespace,
					"pod":       pod.Name,
					"container": container,
					"type":      "minor",
				})
				exporter.OutdatedMetric.Delete(prometheus.Labels{
					"namespace": pod.Namespace,
					"pod":       pod.Name,
					"container": container,
					"type":      "patch",
				})
			}
			delete(e.podContainers, fmt.Sprintf("%s/%s", pod.Namespace, pod.Name))
			return nil
		}

		var pinMode version.PinMode
		switch pod.Annotations["outdated-images.patrick246.de/pin-mode"] {
		case "major":
			pinMode = version.PIN_MAJOR
		case "minor":
			pinMode = version.PIN_MINOR
		default:
			pinMode = version.PIN_NONE
		}

		var containers []string
		for container, image := range pod.Images {
			currentVersion, err := e.tagLister.GetTagOfImage(image)
			if err != nil {
				continue
			}

			imageTags, err := e.tagLister.ListTags(image)
			if err != nil {
				continue
			}
			major, minor, patch, err := e.versionChecker.GetDifference(currentVersion, imageTags, pinMode)
			if err != nil {
				continue
			}

			containers = append(containers, container)

			log.Printf("pod %q container %q is major=%d minor=%d patch=%d behind", pod.Name, container, major, minor, patch)
			exporter.OutdatedMetric.With(prometheus.Labels{
				"namespace": pod.Namespace,
				"pod":       pod.Name,
				"container": container,
				"type":      "major",
			}).Set(float64(major))
			exporter.OutdatedMetric.With(prometheus.Labels{
				"namespace": pod.Namespace,
				"pod":       pod.Name,
				"container": container,
				"type":      "minor",
			}).Set(float64(minor))
			exporter.OutdatedMetric.With(prometheus.Labels{
				"namespace": pod.Namespace,
				"pod":       pod.Name,
				"container": container,
				"type":      "patch",
			}).Set(float64(patch))
		}
		e.podContainers[fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)] = containers
		return nil
	})
}
