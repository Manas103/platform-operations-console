package collector

import (
	"database/sql"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"platformops/collector/fakeupstream"
)

func openTestDB(t *testing.T) *sql.DB {
	dsn := os.Getenv("POC_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://platformops:platformops@127.0.0.1:5432/platformops_console?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE IF EXISTS poc_records`); err != nil {
		t.Fatalf("reset table: %v", err)
	}
	return db
}

func TestCollector_PollOnceNormalizesAllSources(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := NewStore(db)
	if err := store.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	body := []byte(`[{"id":"1","environment":"staging","status":"passed","updated_at":"2026-01-01T00:00:00Z"}]`)
	var endpoints []SourceEndpoint
	for _, src := range AllSources {
		srv := httptest.NewServer(fakeupstream.New(body, 1))
		t.Cleanup(srv.Close)
		endpoints = append(endpoints, SourceEndpoint{Source: src, URL: srv.URL})
	}

	coll := NewCollector(store, endpoints, 10, 10)
	n, called, err := coll.PollOnce()
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if n != len(AllSources) {
		t.Fatalf("wrote %d records, want %d", n, len(AllSources))
	}
	for _, src := range AllSources {
		if !called[src] {
			t.Errorf("source %s: expected first poll to call upstream", src)
		}
		count, err := store.CountRecords(src)
		if err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("source %s: got %d records in store, want 1", src, count)
		}
	}
}

func TestCollector_ReadinessReflectsAllFourSources(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()
	store := NewStore(db)
	if err := store.EnsureSchema(); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(store.Upsert(SourceCIRuns, Record{ID: "1", Environment: "prod", Status: "passed", UpdatedAt: now}))
	must(store.Upsert(SourceIncidents, Record{ID: "1", Environment: "prod", Status: "open", UpdatedAt: now}))
	must(store.Upsert(SourceAlerts, Record{ID: "1", Environment: "prod", Status: "acked", UpdatedAt: now}))
	must(store.Upsert(SourceHostInventory, Record{ID: "1", Environment: "prod", Status: "healthy", UpdatedAt: now}))

	statuses, err := store.Readiness()
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("got %d environments, want 1", len(statuses))
	}
	st := statuses[0]
	if st.Ready {
		t.Fatal("expected not ready: an open incident should block readiness")
	}
	if st.OpenIncidents != 1 {
		t.Fatalf("open incidents = %d, want 1", st.OpenIncidents)
	}

	must(store.Upsert(SourceIncidents, Record{ID: "1", Environment: "prod", Status: "closed", UpdatedAt: now}))
	statuses, err = store.Readiness()
	if err != nil {
		t.Fatal(err)
	}
	if !statuses[0].Ready {
		t.Fatal("expected ready once the incident is closed and every other source is healthy")
	}
}
