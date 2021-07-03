package exporter

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"time"
)

var OutdatedMetric = prometheus.NewGaugeVec(prometheus.GaugeOpts{
	Name:        "pod_image_outdated",
	Help:        "Exports how many major, minor or patch versions a image in a podspec is outdated",
	ConstLabels: nil,
}, []string{"namespace", "pod", "container", "type"})

func init() {
	prometheus.MustRegister(OutdatedMetric)
}

func RunServer(addr string, runCtx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/ready", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
	}))
	server := http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		<-runCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	return server.ListenAndServe()
}
