package metrics

import (
	"fmt"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus metric collectors.
var (
	RestartsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "docker_guardian_restarts_total",
		Help: "Total container restarts by result.",
	}, []string{"container", "result"})

	SkipsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "docker_guardian_skips_total",
		Help: "Total skipped containers by reason.",
	}, []string{"container", "reason"})

	NotificationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "docker_guardian_notifications_total",
		Help: "Total notification sends by service and result.",
	}, []string{"service", "result"})

	EventsProcessedTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "docker_guardian_events_processed_total",
		Help: "Total Docker events processed by action.",
	}, []string{"action"})

	UnhealthyContainers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "docker_guardian_unhealthy_containers",
		Help: "Current number of unhealthy containers.",
	})

	CircuitOpenContainers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "docker_guardian_circuit_open_containers",
		Help: "Number of containers with open circuit breakers.",
	})

	EventStreamConnected = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "docker_guardian_event_stream_connected",
		Help: "1 if connected to Docker event stream, 0 otherwise.",
	})

	RestartDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "docker_guardian_restart_duration_seconds",
		Help:    "Time taken to restart a container.",
		Buckets: prometheus.DefBuckets,
	}, []string{"container"})

	EventProcessingDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "docker_guardian_event_processing_duration_seconds",
		Help:    "Time taken to process a Docker event.",
		Buckets: prometheus.DefBuckets,
	})
)

func init() {
	prometheus.MustRegister(
		RestartsTotal,
		SkipsTotal,
		NotificationsTotal,
		EventsProcessedTotal,
		UnhealthyContainers,
		CircuitOpenContainers,
		EventStreamConnected,
		RestartDuration,
		EventProcessingDuration,
	)
}

// Serve starts the Prometheus metrics HTTP server on the given port.
// Returns immediately; the server runs in the background.
// If port is 0, metrics are disabled and this is a no-op.
func Serve(port int) {
	if port == 0 {
		return
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	go func() {
		addr := fmt.Sprintf(":%d", port)
		if err := http.ListenAndServe(addr, mux); err != nil { //nolint:gosec // Metrics endpoint, intentionally unauthenticated
			fmt.Printf("metrics server error: %v\n", err)
		}
	}()
}
