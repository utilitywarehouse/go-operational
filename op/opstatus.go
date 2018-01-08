package op

import (
	"github.com/prometheus/client_golang/prometheus"
	"strings"
)

const (
	healthy   = "healthy"
	degraded  = "degraded"
	unhealthy = "unhealthy"
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

// AddCheckerWithMetrics adds a function that can check the applications health and meter the outcomes of the checks
// Multiple checkers are allowed.  The checker functions should be capable of
// being called concurrently (with each other and with themselves).
func (s *Status) AddCheckerWithMetrics(name string, checkerFunc func(cr *CheckResponse)) *Status {
	checkCounterName := strings.Replace(name, " ", "_", -1)
	checkCounterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: checkCounterName,
		Help: "Counts the number of times the check was healthy",
	}, []string{"healthcheck_result"})
	// healthyCheckCounter := prometheus.NewCounter(prometheus.CounterOpts{
	// 	Name: checkCounterName + "_" + healthy,
	// 	Help: "Counts the number of times the check was healthy",
	// })
	// unhealthyCheckCounter := prometheus.NewCounter(prometheus.CounterOpts{
	// 	Name: checkCounterName + "_" + unhealthy,
	// 	Help: "Counts the number of times the check was unhealthy",
	// })
	// degradedhealthCheckCounter := prometheus.NewCounter(prometheus.CounterOpts{
	// 	Name: checkCounterName + "_" + degraded,
	// 	Help: "Counts the number of times the check was degraded",
	// })
	s.AddMetrics(checkCounterVec)

	s.checkers = append(s.checkers, checker{name, checkerFunc, checkCounterVec})
	return s
}

// AddChecker adds a function that can check the applications health.
// Multiple checkers are allowed.  The checker functions should be capable of
// being called concurrently (with each other and with themselves).
func (s *Status) AddChecker(name string, checkerFunc func(cr *CheckResponse)) *Status {
	s.checkers = append(s.checkers, checker{name, checkerFunc, nil})
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

// Check returns the current health state of the application.
func (s *Status) Check() HealthResult {
	hr := HealthResult{
		Name:         s.name,
		Description:  s.description,
		CheckResults: make([]healthResultEntry, len(s.checkers)),
	}

	for i, checker := range s.checkers {
		var cr CheckResponse
		checker.checkFunc(&cr)
		hr.CheckResults[i] = healthResultEntry{
			Name:   checker.name,
			Health: cr.health,
			Output: cr.output,
			Action: cr.action,
			Impact: cr.impact,
		}
		updateCheckMetrics(checker, cr)
	}

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

func updateCheckMetrics(checker checker, cr CheckResponse) {
	if checker.checkCounter != nil {
		checker.checkCounter.WithLabelValues(cr.health).Inc()
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
	name        string
	description string
	owners      []owner
	links       []link
	revision    string
	checkers    []checker
	ready       func() bool
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
	name         string
	checkFunc    func(resp *CheckResponse)
	checkCounter *prometheus.CounterVec
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
