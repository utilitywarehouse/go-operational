package op

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthCheckWithMetrics(t *testing.T) {
	assert := assert.New(t)

	hc := NewStatus("my app", "app description").
		AddChecker("check mongo", func(cr *CheckResponse) {
			cr.Healthy("check command completed ok")
		}).
		AddChecker("check kafka", func(cr *CheckResponse) {
			cr.Unhealthy("thing failed", "fix the thing", "very bad")
		}).
		AddChecker("check api", func(cr *CheckResponse) {
			cr.Degraded("thing failed", "fix the thing")
		}).WithInstrumentedChecks()

	expected := HealthResult{
		Name:        "my app",
		Description: "app description",
		Health:      "unhealthy",
		CheckResults: []healthResultEntry{
			{
				Name:   "check mongo",
				Health: "healthy",
				Output: "check command completed ok",
				Action: "",
				Impact: "",
			},
			{
				Name:   "check kafka",
				Health: "unhealthy",
				Output: "thing failed",
				Action: "fix the thing",
				Impact: "very bad",
			},
			{
				Name:   "check api",
				Health: "degraded",
				Output: "thing failed",
				Action: "fix the thing",
				Impact: "",
			},
		},
	}

	result := hc.Check()
	assert.Equal(expected, result)
	mfs, _ := prometheus.DefaultGatherer.Gather()
	assertMetricLabelsAndValue(t, mfs, "check_mongo", healthy, 1)
	assertMetricLabelsAndValue(t, mfs, "check_mongo", degraded, 0)
	assertMetricLabelsAndValue(t, mfs, "check_mongo", unhealthy, 0)

	assertMetricLabelsAndValue(t, mfs, "check_kafka", healthy, 0)
	assertMetricLabelsAndValue(t, mfs, "check_kafka", degraded, 0)
	assertMetricLabelsAndValue(t, mfs, "check_kafka", unhealthy, 1)

	assertMetricLabelsAndValue(t, mfs, "check_api", healthy, 0)
	assertMetricLabelsAndValue(t, mfs, "check_api", degraded, 1)
	assertMetricLabelsAndValue(t, mfs, "check_api", unhealthy, 0)

}

func assertMetricLabelsAndValue(t *testing.T, mfs []*dto.MetricFamily, checkname string, outcome string, value int) {
	for _, mf := range mfs {
		if mf.GetName() == healthcheckStatus && mf.GetType() == dto.MetricType_GAUGE {
			for _, metric := range mf.Metric {
				matchedName, matchedResult := false, false
				for _, metricLabel := range metric.GetLabel() {
					if metricLabel.GetName() == healthcheckName && metricLabel.GetValue() == checkname {
						matchedName = true
					}
					if metricLabel.GetName() == healthcheckResult && metricLabel.GetValue() == outcome {
						matchedResult = true
					}
				}
				if matchedName && matchedResult {
					assert.Equal(t, float64(value), metric.GetGauge().GetValue())
					return
				}
			}
		}
	}
	assert.Fail(t, "Expected counter to match labels and count, but nt")
}
