package collector

import (
	"net/http/httptest"
	"testing"
	"time"

	"platformops/collector/fakeupstream"
)

func TestCachedClient_ServesFromCacheWithinMaxAge(t *testing.T) {
	upstream := fakeupstream.New([]byte(`[]`), 1) // max-age=1s
	srv := httptest.NewServer(upstream)
	defer srv.Close()

	client := NewCachedClient(NewTokenBucket(100, 100))
	_, called1, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if !called1 {
		t.Fatal("first fetch should call upstream")
	}
	_, called2, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if called2 {
		t.Fatal("second fetch within max-age should be served from cache, not call upstream")
	}
	if upstream.RequestCount() != 1 {
		t.Fatalf("upstream saw %d requests, want 1", upstream.RequestCount())
	}
}

func TestCachedClient_RevalidatesAfterMaxAgeWith304(t *testing.T) {
	upstream := fakeupstream.New([]byte(`[]`), 0) // max-age=0, always revalidate
	srv := httptest.NewServer(upstream)
	defer srv.Close()

	client := NewCachedClient(NewTokenBucket(100, 100))
	if _, _, err := client.Get(srv.URL); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	_, called, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected a real network call for revalidation")
	}
	if client.NotModified != 1 {
		t.Fatalf("expected 1 not-modified response, got %d", client.NotModified)
	}
	if upstream.RequestCount() != 2 {
		t.Fatalf("upstream saw %d requests, want 2", upstream.RequestCount())
	}
}

// TestCachedClient_SurvivesRateLimitStormWithZeroDrops is the "replayed rate-limit storm
// dropped 0 records" claim at the client layer: a burst of 429s must never cause Get to give
// up, only to retry with bounded backoff until the upstream recovers.
func TestCachedClient_SurvivesRateLimitStormWithZeroDrops(t *testing.T) {
	body := []byte(`[{"id":"1","environment":"prod","status":"passed","updated_at":"2026-01-01T00:00:00Z"}]`)
	upstream := fakeupstream.New(body, 0)
	srv := httptest.NewServer(upstream)
	defer srv.Close()

	upstream.StartStorm(4) // first 4 requests get 429; MaxRetries defaults to 6

	client := NewCachedClient(NewTokenBucket(100, 100))
	client.BaseBackoff = 1 * time.Millisecond
	client.MaxBackoff = 20 * time.Millisecond

	data, called, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("expected the storm to be survived, got error: %v", err)
	}
	if !called {
		t.Fatal("expected a real network call")
	}
	if len(data) == 0 {
		t.Fatal("expected a non-empty body after the storm cleared")
	}
	if client.Dropped != 0 {
		t.Fatalf("expected 0 dropped, got %d", client.Dropped)
	}
}
