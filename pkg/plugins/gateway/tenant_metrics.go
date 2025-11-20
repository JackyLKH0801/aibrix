package gateway

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	legacyTenantFallbacks = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "aibrix_gateway_legacy_tenant_fallback_total",
		Help: "Total requests routed through the legacy default tenant",
	})

	tenantConflictTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aibrix_gateway_tenant_conflicts_total",
		Help: "Total requests rejected due to conflicting tenant metadata",
	}, []string{"field"})

	requestTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "aibrix_gateway_requests_total",
		Help: "Total requests processed by the gateway",
	}, []string{"tenant_id", "model_id"})

	metricsOnce sync.Once
)

func registerTenantMetrics() {
	metricsOnce.Do(func() {
		prometheus.MustRegister(legacyTenantFallbacks)
		prometheus.MustRegister(tenantConflictTotal)
		prometheus.MustRegister(requestTotal)
	})
}

func recordLegacyTenantFallback() {
	registerTenantMetrics()
	legacyTenantFallbacks.Inc()
}

func recordTenantConflict(field string) {
	registerTenantMetrics()
	tenantConflictTotal.WithLabelValues(field).Inc()
}

func recordRequest(tenantID, modelID string) {
	registerTenantMetrics()
	requestTotal.WithLabelValues(tenantID, modelID).Inc()
}
