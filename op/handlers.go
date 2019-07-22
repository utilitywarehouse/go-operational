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
	if len(hc.checkers) == 0 {
		return http.NotFoundHandler()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		if err := newEncoder(w).Encode(hc.Check()); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
	})
}

func newReadyHandler(hc *Status) http.Handler {
	if hc.ready == nil {
		return http.NotFoundHandler()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

// NewHandler created a new HTTP handler that should be mapped to "/__/".
// It will create all the standard endpoints it can based on how the OpStatus
// is configured.
func NewHandler(os *Status) http.Handler {
	m := http.NewServeMux()
	m.Handle("/__/about", newAboutHandler(os))
	m.Handle("/__/health", newHealthCheckHandler(os))
	m.Handle("/__/ready", newReadyHandler(os))
	m.Handle("/__/metrics", promhttp.Handler())

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
