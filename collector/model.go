package collector

import "time"

// Record is the minimal shape every one of the 4 upstream sources returns: an id, the
// environment it belongs to, a free-form status/body, and the time the upstream last changed
// it. The collector does not need to understand each source's full domain shape, only enough
// to normalize into Postgres and compute readiness.
type Record struct {
	ID          string    `json:"id"`
	Environment string    `json:"environment"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Source names the 4 operations APIs the collector integrates, matching the resume claim
// (CI runs, alerts, incidents, host inventory).
type Source string

const (
	SourceCIRuns        Source = "ci_runs"
	SourceAlerts        Source = "alerts"
	SourceIncidents     Source = "incidents"
	SourceHostInventory Source = "host_inventory"
)

var AllSources = []Source{SourceCIRuns, SourceAlerts, SourceIncidents, SourceHostInventory}
