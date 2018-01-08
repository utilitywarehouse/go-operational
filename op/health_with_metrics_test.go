package op

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/stretchr/testify/assert"
	"testing"
)

func TestHealthCheckWithMetrics(t *testing.T) {
	assert := assert.New(t)

	hc := NewStatus("my app", "app description").
		AddCheckerWithMetrics("check the foo bar", func(cr *CheckResponse) {
			cr.Healthy("check command completed ok")
		}).
		AddCheckerWithMetrics("check the unhealthy one", func(cr *CheckResponse) {
			cr.Unhealthy("thing failed", "fix the thing", "very bad")
		}).
		AddCheckerWithMetrics("check the bar baz", func(cr *CheckResponse) {
			cr.Degraded("thing failed", "fix the thing")
		})

	expected := HealthResult{
		Name:        "my app",
		Description: "app description",
		Health:      "unhealthy",
		CheckResults: []healthResultEntry{
			{
				Name:   "check the foo bar",
				Health: "healthy",
				Output: "check command completed ok",
				Action: "",
				Impact: "",
			},
			{
				Name:   "check the unhealthy one",
				Health: "unhealthy",
				Output: "thing failed",
				Action: "fix the thing",
				Impact: "very bad",
			},
			{
				Name:   "check the bar baz",
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
	checkAssertion(t, mfs, "check_the_foo_bar", healthy, 1)
	checkAssertion(t, mfs, "check_the_bar_baz", degraded, 1)
	checkAssertion(t, mfs, "check_the_unhealthy_one", unhealthy, 1)

	// Check that the metrics are actually incrementing
	hc.Check()
	mfs, _ = prometheus.DefaultGatherer.Gather()
	checkAssertion(t, mfs, "check_the_foo_bar", healthy, 2)
	checkAssertion(t, mfs, "check_the_bar_baz", degraded, 2)
	checkAssertion(t, mfs, "check_the_unhealthy_one", unhealthy, 2)
}

func checkAssertion(t *testing.T, mfs []*dto.MetricFamily, checkname string, outcome string, value int) {
	for _, mf := range mfs {
		if mf.GetName() == checkname && mf.GetType() == dto.MetricType_COUNTER {
			for _, x := range mf.Metric {
				assert.Equal(t, float64(value), x.GetCounter().GetValue())

				for _, label := range x.Label {
					if *label.Name == "healthcheck_result" {
						assert.Equal(t, outcome, *label.Value)
					}
				}
			}
		}
	}
}
