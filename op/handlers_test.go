package op

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestAboutHandler(t *testing.T) {
	expectedAbout := `{
    "name": "name",
    "description": "desc",
    "owners": [
      {
        "name": "owner",
        "slack": "#slack"
      }
    ],
    "links": [
      {
        "description": "link-desc1",
        "url": "http://url1/"
      },
      {
        "description": "link-desc2",
        "url": "http://url2/"
      }
    ],
    "build-info": {
      "revision": "revision"
    }
  }`
	assert := assert.New(t)

	h := newAboutHandler(
		NewStatus("name", "desc").
			AddOwner("owner", "#slack").
			SetRevision("revision").
			AddLink("link-desc1", "http://url1/").
			AddLink("link-desc2", "http://url2/"),
	)

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("expected status %v but got %v", http.StatusOK, status)
	}

	assert.Equal(expectedAbout, rr.Body.String())
}

var expectedHealth = `{
    "name": "name",
    "description": "desc",
    "health": "unhealthy",
    "checks": [
      {
        "name": "check1",
        "health": "unhealthy",
        "output": "output1",
        "action": "action1",
        "impact": "impact1"
      },
      {
        "name": "check2",
        "health": "degraded",
        "output": "output2",
        "action": "action2"
      },
      {
        "name": "check3",
        "health": "healthy",
        "output": "output3"
      }
    ]
  }
`

func TestHealthCheckHandler(t *testing.T) {
	assert := assert.New(t)

	h := newHealthCheckHandler(
		NewStatus("name", "desc").
			AddChecker("check1", func(cr *CheckResponse) {
				cr.Unhealthy("output1", "action1", "impact1")
			}).
			AddChecker("check2", func(cr *CheckResponse) {
				cr.Degraded("output2", "action2")
			}).
			AddChecker("check3", func(cr *CheckResponse) {
				cr.Healthy("output3")
			}),
	)

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("expected status %v but got %v", http.StatusOK, status)
	}

	assert.Equal(expectedHealth, rr.Body.String())
}

func TestHealthCheckHandler_PostServeNewChecker(t *testing.T) {
	assert := assert.New(t)

	st := NewStatus("name", "desc")
	h := newHealthCheckHandler(st)

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	assert.Equal(http.StatusNotFound, rr.Code, "Expected 404 when no checkers are registered")

	st.AddChecker("check1", func(cr *CheckResponse) {
		cr.Healthy("output1")
	})

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)

	assert.Equal(http.StatusOK, rr2.Code, "Expected 200 after adding a healthy checker")
}

func TestReadyHandlerReady(t *testing.T) {
	assert := assert.New(t)

	h := newReadyHandler(&Status{ready: func() bool { return true }})

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("expected status %v but got %v", http.StatusOK, status)
	}

	assert.Equal("ready\n", rr.Body.String())
}

func TestReadyHandlerNotReady(t *testing.T) {
	assert := assert.New(t)

	h := newReadyHandler(&Status{ready: func() bool { return false }})

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("expected status %v but got %v", http.StatusServiceUnavailable, status)
	}

	assert.Empty(rr.Body.String())
}

func TestReadyHandlerNone(t *testing.T) {
	h := newReadyHandler(NewStatus("", "").ReadyNone())

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("expected status %v but got %v", http.StatusNotFound, status)
	}
}

func TestReadyHandlerNone_PostServeHandlerReady(t *testing.T) {
	st := NewStatus("", "").ReadyNone()
	h := newReadyHandler(st)

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusNotFound, rr.Code, "Expected 404 when readiness is set to none")

	st.ReadyAlways()

	rr2 := httptest.NewRecorder()
	h.ServeHTTP(rr2, req)
	assert.Equal(t, http.StatusOK, rr2.Code, "Expected 200 after changing readiness to always")
}

func TestReadyHandlerDefaults(t *testing.T) {
	h := newReadyHandler(&Status{})

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusNotFound {
		t.Errorf("expected status %v but got %v", http.StatusNotFound, status)
	}
}

func TestMetricsHandler(t *testing.T) {
	assert := assert.New(t)
	metric := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_metric",
		Help: "Dummy counter",
	})

	s := &Status{}
	s.AddMetrics(metric)
	h := NewHandler(s)

	metric.Inc()

	req, err := http.NewRequest("GET", "/__/metrics", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)
	assert.Equal(http.StatusOK, rr.Code, "Response status should be 200")
	assert.True(strings.Contains(rr.Body.String(), "test_metric 1\n"), "Metrics response should contain dummy metric")
}
