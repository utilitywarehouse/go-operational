package op

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func newHealthCheckHandler(hc *Status) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(hc.checkers) == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		if err := newEncoder(w).Encode(hc.Check()); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

func newReadyHandler(hc *Status) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hc.ready == nil {
			http.NotFound(w, r)
			return
		}

		if hc.ready() {
			w.Header().Add("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "ready\n")
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
}

func newAboutHandler(os *Status) http.Handler {
	j, err := json.MarshalIndent(os.About(), "  ", "  ")
	if err != nil {
		panic(err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(j)
		if err != nil {
			log.Println("failed to write about response")
		}
	})
}

type opsHandler struct {
	aboutHandler       http.Handler
	healthCheckHandler http.Handler
	readyHandler       http.Handler
	prometheusHandler  http.Handler
}

type Option func(*opsHandler)

func WithAboutHandler(h http.Handler) Option {
	return func(o *opsHandler) {
		o.aboutHandler = h
	}
}

func WithHealthCheckHandler(h http.Handler) Option {
	return func(o *opsHandler) {
		o.healthCheckHandler = h
	}
}

func WithReadyHandler(h http.Handler) Option {
	return func(o *opsHandler) {
		o.readyHandler = h
	}
}

func WithPrometheusHandler(h http.Handler) Option {
	return func(o *opsHandler) {
		o.prometheusHandler = h
	}
}

// NewHandler created a new HTTP handler that should be mapped to "/__/".
// It will create all the standard endpoints it can based on how the OpStatus
// is configured.
func NewHandler(os *Status, opts ...Option) http.Handler {
	m := http.NewServeMux()

	ops := &opsHandler{
		aboutHandler:       newAboutHandler(os),
		healthCheckHandler: newHealthCheckHandler(os),
		readyHandler:       newReadyHandler(os),
		prometheusHandler:  promhttp.Handler(),
	}

	for _, opt := range opts {
		opt(ops)
	}

	m.Handle("/__/about", ops.aboutHandler)
	m.Handle("/__/health", ops.healthCheckHandler)
	m.Handle("/__/ready", ops.readyHandler)
	m.Handle("/__/metrics", ops.prometheusHandler)

	// Overload default mux in order to stop pprof binding handlers to it
	http.DefaultServeMux = http.NewServeMux()

	// Register PPROF handlers
	m.Handle("/__/extended/pprof/", http.HandlerFunc(pprof.Index))
	m.Handle("/__/extended/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	m.Handle("/__/extended/pprof/profile", http.HandlerFunc(pprof.Profile))
	m.Handle("/__/extended/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	m.Handle("/__/extended/pprof/trace", http.HandlerFunc(pprof.Trace))
	m.Handle("/__/extended/pprof/goroutine", pprof.Handler("goroutine"))
	m.Handle("/__/extended/pprof/heap", pprof.Handler("heap"))
	m.Handle("/__/extended/pprof/threadcreate", pprof.Handler("threadcreate"))
	m.Handle("/__/extended/pprof/block", pprof.Handler("block"))
	m.Handle("/__/extended/pprof/mutex", pprof.Handler("mutex"))
	m.Handle("/__/extended/pprof/allocs", pprof.Handler("allocs"))

	return m
}
