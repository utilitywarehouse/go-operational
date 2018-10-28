package op

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	healthy           = "healthy"
	degraded          = "degraded"
	unhealthy         = "unhealthy"
	healthcheckName   = "healthcheck_name"
	healthcheckResult = "healthcheck_result"
	healthcheckStatus = "healthcheck_status"
)

// NewStatus returns a new Status, given an application or service name and
// description.
func NewStatus(name, description string) *Status {
	return &Status{name: name, description: description}
}

// AddOwner adds an owner entry. Each can have a name, a slack channel or both.
// Multiple owner entries are allowed.
func (s *Status) AddOwner(name, slack string) *Status {
	s.owners = append(s.owners, owner{name: name, slack: slack})
	return s
}

// AddLink adds a URL link. Multiple are allowed and each should have a brief
// description.
func (s *Status) AddLink(description, url string) *Status {
	s.links = append(s.links, link{description: description, url: url})
	return s
}

// SetRevision sets the source control revision string, typically a git hash.
func (s *Status) SetRevision(revision string) *Status {
	s.revision = revision
	return s
}

// AddChecker adds a function that can check the applications health.
// Multiple checkers are allowed.  The checker functions should be capable of
// being called concurrently (with each other and with themselves).
func (s *Status) AddChecker(name string, checkerFunc func(cr *CheckResponse)) *Status {
	s.checkers = append(s.checkers, checker{name, checkerFunc})
	return s
}

// RemoveCheckers will remove health check functions added by AddChecker.
// If multiple checks have been added with the same name, these will all be removed.
func (s *Status) RemoveCheckers(name string) *Status {
	var checkers []checker
	for _, ch := range s.checkers {
		if ch.name != name {
			checkers = append(checkers, ch)
		}
	}
	s.checkers = checkers
	return s
}

// AddMetrics registers prometheus metrics to be exopsed on the /__/metrics endpoint
// Adding the same metric twice will result in a panic
func (s *Status) AddMetrics(cs ...prometheus.Collector) *Status {
	prometheus.MustRegister(cs...)
	return s
}

// ReadyNone indicates that this application doesn't expose a concept of
// readiness.
func (s *Status) ReadyNone() *Status {
	s.ready = nil
	return s
}

// ReadyAlways indicates that this application is always ready, typically if it
// has no external systems to depend upon.
func (s *Status) ReadyAlways() *Status {
	s.ready = func() bool { return true }
	return s
}

// ReadyNever indicates that this application is never ready. Typically this is
// only useful in testing.
func (s *Status) ReadyNever() *Status {
	s.ready = func() bool { return false }
	return s
}

// ReadyUseHealthCheck indicates that the readiness of this application should
// re-use the health check. If the health is "ready" or "degraded" the
// application is considered ready.
func (s *Status) ReadyUseHealthCheck() *Status {
	s.ready = func() bool {
		switch s.Check().Health {
		case healthy:
			return true
		case degraded:
			return true
		default:
			return false
		}
	}
	return s
}

// Ready allows specifying any readiness function.
func (s *Status) Ready(f func() bool) *Status {
	s.ready = f
	return s
}

// Check returns the current health state of the application. Each checker is
// run concurrently.
func (s *Status) Check() HealthResult {
	hr := HealthResult{
		Name:         s.name,
		Description:  s.description,
		CheckResults: make([]healthResultEntry, len(s.checkers)),
	}

	var wg sync.WaitGroup
	wg.Add(len(s.checkers))

	for i, ch := range s.checkers {
		go func(i int, ch checker) {
			defer wg.Done()

			var cr CheckResponse
			ch.checkFunc(&cr)
			hr.CheckResults[i] = healthResultEntry{
				Name:   ch.name,
				Health: cr.health,
				Output: cr.output,
				Action: cr.action,
				Impact: cr.impact,
			}
			s.updateCheckMetrics(ch, cr)
		}(i, ch)
	}

	wg.Wait()

	var seenHealthy, seenDegraded, seenUnhealthy bool
	for _, hcr := range hr.CheckResults {
		switch hcr.Health {
		case healthy:
			seenHealthy = true
		case degraded:
			seenDegraded = true
		case unhealthy:
			seenUnhealthy = true
		}
	}

	switch {
	case seenUnhealthy:
		hr.Health = unhealthy
	case seenDegraded:
		hr.Health = degraded
	case seenHealthy:
		hr.Health = healthy
	default:
		// We have no health checks. Assume unhealthy.
		hr.Health = unhealthy
	}

	return hr
}

// WithInstrumentedChecks enables the outcome of healthchecks to be instrumented as a counter
func (s *Status) WithInstrumentedChecks() *Status {
	checkGaugeVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: healthcheckStatus,
		Help: "Meters the healthcheck status based for each check and for each result",
	}, []string{healthcheckName, healthcheckResult})
	s.checkResultGauge = checkGaugeVec
	prometheus.MustRegister(s.checkResultGauge)
	return s
}

func safeMetricName(checkName string) string {
	x := ""
	for i, b := range checkName {
		if !((b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' || b == ':' || (b >= '0' && b <= '9' && i > 0)) {
			x = x + "_"
		} else {
			x = x + string(b)
		}
	}
	return x
}

func (s *Status) updateCheckMetrics(checker checker, cr CheckResponse) {
	if s.checkResultGauge != nil {
		possibleStatuses := []string{healthy, unhealthy, degraded}
		for _, status := range possibleStatuses {
			if cr.health == status {
				s.checkResultGauge.With(map[string]string{healthcheckName: safeMetricName(checker.name), healthcheckResult: status}).Set(1)
				continue
			}
			s.checkResultGauge.With(map[string]string{healthcheckName: safeMetricName(checker.name), healthcheckResult: status}).Set(0)
		}
	}
}

// About returns static information about this application or service.
func (s *Status) About() AboutResponse {
	about := AboutResponse{
		Name:        s.name,
		Description: s.description,
		BuildInfo:   buildInfoResponse{Revision: s.revision},
	}

	for _, l := range s.links {
		about.Links = append(about.Links, linkResponse{l.description, l.url})
	}
	for _, o := range s.owners {
		about.Owners = append(about.Owners, ownerResponse{o.name, o.slack})
	}
	return about
}

// Status represents standard operational information about an application,
// including how to establish dynamic information such as health or readiness.
type Status struct {
	name             string
	description      string
	owners           []owner
	links            []link
	revision         string
	checkers         []checker
	ready            func() bool
	checkResultGauge *prometheus.GaugeVec
}

type owner struct {
	name  string
	slack string
}

type link struct {
	description string
	url         string
}

// AboutResponse represents the static "about" information for an application
// as described in the UW operation endpoints spec.  When serialised to JSON
// it is compiant with that spec.
type AboutResponse struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Owners      []ownerResponse   `json:"owners"`
	Links       []linkResponse    `json:"links,omitempty"`
	BuildInfo   buildInfoResponse `json:"build-info"`
}

type ownerResponse struct {
	Name  string `json:"name"`
	Slack string `json:"slack,omitempty"`
}

type linkResponse struct {
	Description string `json:"description"`
	URL         string `json:"url"`
}

type buildInfoResponse struct {
	Revision string `json:"revision"`
}

type checker struct {
	name      string
	checkFunc func(resp *CheckResponse)
}

// CheckResponse is used by a health check function to allow it to indicate
// the result of the check be calling appropriate methods.
type CheckResponse struct {
	health string
	output string
	action string
	impact string
}

// Healthy indicates that the check reports good health. The output of the check
// command should be provided.
func (cr *CheckResponse) Healthy(output string) {
	cr.health = healthy
	cr.output = output
	cr.action = ""
	cr.impact = ""
}

// Degraded indicates that the check reports degraded health. In addition to
// the output of the check output, the recommended action should be provided.
func (cr *CheckResponse) Degraded(output, action string) {
	cr.health = degraded
	cr.output = output
	cr.action = action
	cr.impact = ""
}

// Unhealthy indicates an unhealthy check. The output, a recommended action,
// and a brief description of the impact should be provided.
func (cr *CheckResponse) Unhealthy(output, action, impact string) {
	cr.health = unhealthy
	cr.output = output
	cr.action = action
	cr.impact = impact
}

// HealthResult represents the current "health" information for an application
// as described in the UW operation endpoints spec.  When serialised to JSON
// it is compiant with that spec.
type HealthResult struct {
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Health       string              `json:"health"`
	CheckResults []healthResultEntry `json:"checks"`
}

type healthResultEntry struct {
	Name   string `json:"name"`
	Health string `json:"health"`
	Output string `json:"output"`
	Action string `json:"action,omitempty"`
	Impact string `json:"impact,omitempty"`
}
