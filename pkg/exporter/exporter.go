package exporter

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/patrick246/k8s-outdated-image-exporter/pkg/evaluation"
)

type Collector struct {
	evaluator *evaluation.Evaluator
}

func NewCollector(evaluator *evaluation.Evaluator) *Collector {
	return &Collector{
		evaluator: evaluator,
	}
}

func (c *Collector) Describe(descs chan<- *prometheus.Desc) {
	return
}

func (c *Collector) Collect(metrics chan<- prometheus.Metric) {
	for _, metric := range c.evaluator.Metrics() {
		metrics <- metric
	}
}

func RunServer(addr string) (func() error, error) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/ready", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
	}))

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	server := http.Server{
		Handler: mux,
	}

	go func() {
		err := server.Serve(lis)
		if err != nil {
			return
		}
	}()

	return func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		return server.Shutdown(shutdownCtx)
	}, nil
}
