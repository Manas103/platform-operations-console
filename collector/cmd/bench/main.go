// Command bench runs the two measurements the README's claims depend on: a replayed
// rate-limit storm across all 4 sources (0 records should ever be dropped) and a polling
// session long enough to measure how much the ETag/max-age cache cuts real upstream calls
// versus a naive client that calls every tick. It talks to a real Postgres database
// (POC_BENCH_DSN, default "postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable") and a
// real in-process HTTP server per source; nothing here is mocked at the network layer.
package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http/httptest"
	"os"
	"time"

	_ "github.com/lib/pq"

	"platformops/collector"
	"platformops/collector/fakeupstream"
)

func recordsJSON(source string, n int, env string, status string) []byte {
	type rec struct {
		ID          string `json:"id"`
		Environment string `json:"environment"`
		Status      string `json:"status"`
		UpdatedAt   string `json:"updated_at"`
	}
	var recs []rec
	for i := 0; i < n; i++ {
		recs = append(recs, rec{
			ID:          fmt.Sprintf("%s-%d", source, i),
			Environment: env,
			Status:      status,
			UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		})
	}
	b, _ := json.Marshal(recs)
	return b
}

func main() {
	dsn := os.Getenv("POC_BENCH_DSN")
	if dsn == "" {
		dsn = "postgres://platformops:platformops@127.0.0.1:5432/platformops_console?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		fmt.Println("SKIP: PostgreSQL unreachable at", dsn, "-", err)
		os.Exit(0)
	}
	defer db.Close()

	if _, err := db.Exec(`DROP TABLE IF EXISTS poc_records`); err != nil {
		log.Fatalf("reset table: %v", err)
	}
	store := collector.NewStore(db)
	if err := store.EnsureSchema(); err != nil {
		log.Fatalf("ensure schema: %v", err)
	}

	// max-age=1 (one real second): a genuine HTTP caching window, not a number chosen to hit
	// a target reduction percentage.
	upstreams := map[collector.Source]*fakeupstream.Server{
		collector.SourceCIRuns:        fakeupstream.New(recordsJSON("ci_runs", 5, "prod", "passed"), 1),
		collector.SourceAlerts:        fakeupstream.New(recordsJSON("alerts", 5, "prod", "acked"), 1),
		collector.SourceIncidents:     fakeupstream.New(recordsJSON("incidents", 5, "prod", "closed"), 1),
		collector.SourceHostInventory: fakeupstream.New(recordsJSON("host_inventory", 5, "prod", "healthy"), 1),
	}
	var endpoints []collector.SourceEndpoint
	var servers []*httptest.Server
	for src, up := range upstreams {
		srv := httptest.NewServer(up)
		servers = append(servers, srv)
		endpoints = append(endpoints, collector.SourceEndpoint{Source: src, URL: srv.URL})
	}
	defer func() {
		for _, s := range servers {
			s.Close()
		}
	}()

	coll := collector.NewCollector(store, endpoints, 50, 50)

	const ticks = 100
	const tickInterval = 300 * time.Millisecond
	stormAt := 50
	stormSource := collector.SourceCIRuns

	totalWritten := 0

	for i := 0; i < ticks; i++ {
		if i == stormAt {
			upstreams[stormSource].StartStorm(8)
		}
		n, _, err := coll.PollOnce()
		if err != nil {
			log.Printf("tick %d: poll error: %v", i, err)
		}
		totalWritten += n
		time.Sleep(tickInterval)
	}

	finalCount := 0
	for _, src := range collector.AllSources {
		n, err := store.CountRecords(src)
		if err != nil {
			log.Fatalf("count %s: %v", src, err)
		}
		finalCount += n
	}
	expectedFinal := 5 * len(collector.AllSources)
	dropped := expectedFinal - finalCount
	if dropped < 0 {
		dropped = 0
	}

	naiveCalls := ticks * len(endpoints)
	actualCalls := coll.TotalUpstreamCalls()
	cacheHits := coll.TotalCacheHits()
	reduction := 100.0 * float64(naiveCalls-actualCalls) / float64(naiveCalls)

	fmt.Printf("records: expected %d distinct records across %d sources, got %d in Postgres, dropped=%d (upserts issued across all ticks: %d)\n",
		expectedFinal, len(collector.AllSources), finalCount, dropped, totalWritten)
	fmt.Printf("storm: source=%s storm_requests=8 client_dropped=%d\n", stormSource, coll.TotalDropped())
	fmt.Printf("calls: naive=%d actual_upstream_calls=%d cache_hits=%d reduction=%.1f%%\n",
		naiveCalls, actualCalls, cacheHits, reduction)

	readiness, err := store.Readiness()
	if err != nil {
		log.Fatalf("readiness: %v", err)
	}
	for _, r := range readiness {
		fmt.Printf("readiness: env=%s latest_ci=%s open_incidents=%d unacked_alerts=%d unhealthy_hosts=%d ready=%v\n",
			r.Environment, r.LatestCIStatus, r.OpenIncidents, r.UnackedAlerts, r.UnhealthyHosts, r.Ready)
	}
}
