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

	metricsOnce sync.Once
)

func registerTenantMetrics() {
	metricsOnce.Do(func() {
		prometheus.MustRegister(legacyTenantFallbacks)
		prometheus.MustRegister(tenantConflictTotal)
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
