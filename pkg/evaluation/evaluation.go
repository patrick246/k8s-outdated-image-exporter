package evaluation

import (
	"context"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/patrick246/k8s-outdated-image-exporter/pkg/clients"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/tags"
	"github.com/patrick246/k8s-outdated-image-exporter/pkg/version"
)

type ContainerClient interface {
	Listener(ctx context.Context) (<-chan clients.ContainerImage, error)
}

type Evaluator struct {
	containerClient ContainerClient
	tagLister       *tags.TagLister
	versionChecker  *version.Checker
	logger          *slog.Logger

	podContainers map[string][]string
	labelCache    map[string]prometheus.Labels
	metrics       map[string][]Metric
	metricsMutex  sync.RWMutex
}

type Metric struct {
	Labels prometheus.Labels
	Value  float64
}

func NewEvaluator(
	tagLister *tags.TagLister,
	versionChecker *version.Checker,
	containerClient ContainerClient,
	logger *slog.Logger,
) (*Evaluator, error) {
	return &Evaluator{
		containerClient: containerClient,
		tagLister:       tagLister,
		versionChecker:  versionChecker,
		logger:          logger,

		labelCache: map[string]prometheus.Labels{},
		metrics:    map[string][]Metric{},
	}, nil
}

func (e *Evaluator) Run(ctx context.Context) error {
	containerImages, err := e.containerClient.Listener(ctx)
	if err != nil {
		return err
	}

	wg := sync.WaitGroup{}

	for range 4 {
		wg.Add(1)

		go func() {
			for containerImage := range containerImages {
				e.logger.Info("next container to check", "name", containerImage.Name, "image", containerImage.Image, "action", containerImage.Action.String())

				switch containerImage.Action {
				case clients.ContainerImageRemoved:
					e.metricsMutex.Lock()
					delete(e.metrics, containerImage.Name)
					e.metricsMutex.Unlock()

				case clients.ContainerImageAdded:
					err = e.handleContainerImageAdded(ctx, containerImage)
					if err != nil {
						e.logger.Error("error handling container image added", "name", containerImage.Name, "image", containerImage.Image, "error", err)
					}
				}
			}

			wg.Done()
		}()
	}

	wg.Wait()

	return nil
}

func (e *Evaluator) handleContainerImageAdded(ctx context.Context, containerImage clients.ContainerImage) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	logger := e.logger.With("name", containerImage.Name, "image", containerImage.Image)

	var pinMode version.PinMode

	switch containerImage.Annotations["outdated-images.patrick246.de/pin-mode"] {
	case "major":
		pinMode = version.PIN_MAJOR
	case "minor":
		pinMode = version.PIN_MINOR
	default:
		pinMode = version.PIN_NONE
	}

	imageKeychain, ok := containerImage.Metadata["DockerKeychain"].(*tags.DockerConfigKeychain)
	if !ok {
		imageKeychain = &tags.DockerConfigKeychain{}
	}

	logger.InfoContext(ctx, "checking container")

	currentVersion, err := e.tagLister.GetTagOfImage(containerImage.Image)
	if err != nil {
		return err
	}

	logger.Debug("fetching image tags")

	imageTags, err := e.tagLister.ListTags(ctx, containerImage.Image, imageKeychain)
	if err != nil {
		return err
	}

	logger.Debug("got image tags", "count", len(imageTags))

	major, minor, patch, err := e.versionChecker.GetDifference(currentVersion, imageTags, pinMode)
	if err != nil {
		return err
	}

	if major != 0 || minor != 0 || patch != 0 {
		logger.InfoContext(ctx, "image outdated", "major", major, "minor", minor, "patch", patch)
	} else {
		logger.InfoContext(ctx, "image up-to-date", "current", currentVersion)
	}

	labels := prometheus.Labels{
		"container": containerImage.Name,
	}

	for labelKey, labelValue := range containerImage.Labels {
		labels[labelKey] = labelValue
	}

	e.metricsMutex.Lock()
	e.metrics[containerImage.Name] = []Metric{{
		Labels: withLabelValues(labels, "type", "major"),
		Value:  float64(major),
	}, {
		Labels: withLabelValues(labels, "type", "minor"),
		Value:  float64(minor),
	}, {
		Labels: withLabelValues(labels, "type", "patch"),
		Value:  float64(patch),
	}}
	e.metricsMutex.Unlock()

	return nil
}

func (e *Evaluator) Metrics() []prometheus.Metric {
	e.metricsMutex.RLock()
	defer e.metricsMutex.RUnlock()

	result := make([]prometheus.Metric, 0, len(e.metrics))

	for _, containerMetrics := range e.metrics {
		for _, metric := range containerMetrics {
			labelKeys := make([]string, 0, len(metric.Labels))
			labelValues := make([]string, 0, len(metric.Labels))

			for key, value := range metric.Labels {
				labelKeys = append(labelKeys, sanitizeLabelKey(key))
				labelValues = append(labelValues, value)
			}

			result = append(result, prometheus.MustNewConstMetric(
				prometheus.NewDesc(
					"container_image_outdated",
					"Exports how many major, minor or patch versions a image in a podspec is outdated",
					labelKeys,
					nil,
				),
				prometheus.GaugeValue,
				metric.Value,
				labelValues...,
			))
		}
	}

	return result
}

func copyLabels(labels prometheus.Labels) prometheus.Labels {
	labelCopy := prometheus.Labels{}

	for key, value := range labels {
		labelCopy[key] = value
	}

	return labelCopy
}

func withLabelValues(labels prometheus.Labels, key, value string) prometheus.Labels {
	labelCopy := copyLabels(labels)

	labelCopy[key] = value

	return labelCopy
}

var illegalLabelCharactersFirst = regexp.MustCompile(`[^a-zA-Z_]`)
var illegalLabelCharacters = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func sanitizeLabelKey(labelKey string) string {
	firstCharacter := illegalLabelCharactersFirst.ReplaceAllString(labelKey[:1], "_")

	var rest string
	if len(labelKey) > 1 {
		rest = illegalLabelCharacters.ReplaceAllString(labelKey[1:], "_")
	}

	return firstCharacter + rest
}
