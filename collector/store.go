package collector

import (
	"database/sql"
	"fmt"
)

// Store normalizes records from all 4 sources into one Postgres table, keyed on (source, id)
// so a re-fetch of an unchanged record is an idempotent no-op rather than a duplicate.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) EnsureSchema() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS poc_records (
    source      TEXT NOT NULL,
    id          TEXT NOT NULL,
    environment TEXT NOT NULL,
    status      TEXT NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (source, id)
)`)
	return err
}

func (s *Store) Upsert(source Source, r Record) error {
	_, err := s.db.Exec(`
INSERT INTO poc_records (source, id, environment, status, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (source, id) DO UPDATE SET
    environment = EXCLUDED.environment,
    status = EXCLUDED.status,
    updated_at = EXCLUDED.updated_at
`, string(source), r.ID, r.Environment, r.Status, r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert %s/%s: %w", source, r.ID, err)
	}
	return nil
}

func (s *Store) CountRecords(source Source) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT count(*) FROM poc_records WHERE source = $1`, string(source)).Scan(&n)
	return n, err
}

// EnvironmentStatus is one row of the per-environment readiness view.
type EnvironmentStatus struct {
	Environment       string
	LatestCIStatus     string
	OpenIncidents      int
	UnackedAlerts      int
	UnhealthyHosts     int
	Ready              bool
}

// Readiness computes, per environment, whether it is clear to deploy to: the latest CI run
// for that environment must be "passed", there must be no open incidents, no unacknowledged
// alerts, and no unhealthy hosts. This is the "per-environment readiness" view the console
// serves alongside the existing per-service deploy state.
func (s *Store) Readiness() ([]EnvironmentStatus, error) {
	rows, err := s.db.Query(`
SELECT DISTINCT environment FROM poc_records ORDER BY environment
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var envs []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		envs = append(envs, e)
	}

	var out []EnvironmentStatus
	for _, env := range envs {
		st := EnvironmentStatus{Environment: env}

		if err := s.db.QueryRow(`
SELECT status FROM poc_records
WHERE source = $1 AND environment = $2
ORDER BY updated_at DESC LIMIT 1
`, string(SourceCIRuns), env).Scan(&st.LatestCIStatus); err != nil && err != sql.ErrNoRows {
			return nil, err
		}

		if err := s.db.QueryRow(`
SELECT count(*) FROM poc_records WHERE source = $1 AND environment = $2 AND status = 'open'
`, string(SourceIncidents), env).Scan(&st.OpenIncidents); err != nil {
			return nil, err
		}

		if err := s.db.QueryRow(`
SELECT count(*) FROM poc_records WHERE source = $1 AND environment = $2 AND status = 'unacked'
`, string(SourceAlerts), env).Scan(&st.UnackedAlerts); err != nil {
			return nil, err
		}

		if err := s.db.QueryRow(`
SELECT count(*) FROM poc_records WHERE source = $1 AND environment = $2 AND status = 'unhealthy'
`, string(SourceHostInventory), env).Scan(&st.UnhealthyHosts); err != nil {
			return nil, err
		}

		st.Ready = st.LatestCIStatus == "passed" && st.OpenIncidents == 0 && st.UnackedAlerts == 0 && st.UnhealthyHosts == 0
		out = append(out, st)
	}
	return out, nil
}
