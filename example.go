package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/utilitywarehouse/go-operational/op"
)

func main() {
	dm := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "dummy_metric",
		Help: "Dummy counter",
	})
	http.Handle("/__/", op.NewHandler(
		op.NewStatus("My application", "application that does stuff").
			AddOwner("team x", "#team-x").
			SetRevision("7470d3dc24ce7876a9fc53ca7934401273a4017a").
			AddChecker("db check", func(cr *op.CheckResponse) { cr.Healthy("dummy db connection check succeesed") }).
			AddMetrics(dm).
			ReadyUseHealthCheck(),
	),
	)

	http.ListenAndServe(":8080", nil)
}
