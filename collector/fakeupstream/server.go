// Package fakeupstream is test-only infrastructure: a small HTTP server standing in for one of
// the 4 real operations APIs (CI runs, alerts, incidents, host inventory), so the collector's
// rate-limiting, backoff and ETag-caching logic can be measured against real HTTP semantics
// (real status codes, real ETag/If-None-Match/Cache-Control headers, a real injectable 429
// storm) without needing a real third-party account.
package fakeupstream

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
)

type Server struct {
	mu          sync.Mutex
	body        []byte
	etag        string
	maxAgeSecs  int
	requestLog  []string

	stormRemaining int32 // atomic: number of remaining requests to answer with 429
}

func New(initialBody []byte, maxAgeSecs int) *Server {
	s := &Server{maxAgeSecs: maxAgeSecs}
	s.SetBody(initialBody)
	return s
}

func (s *Server) SetBody(body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body = body
	sum := sha256.Sum256(body)
	s.etag = `"` + hex.EncodeToString(sum[:8]) + `"`
}

// StartStorm makes the next n requests return 429 Too Many Requests, simulating a rate-limit
// storm the collector's backoff must survive without dropping any record.
func (s *Server) StartStorm(n int) {
	atomic.StoreInt32(&s.stormRemaining, int32(n))
}

func (s *Server) RequestCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.requestLog)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requestLog = append(s.requestLog, r.Method+" "+r.URL.String())
	s.mu.Unlock()

	if remaining := atomic.LoadInt32(&s.stormRemaining); remaining > 0 {
		atomic.AddInt32(&s.stormRemaining, -1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}

	s.mu.Lock()
	etag, body, maxAge := s.etag, s.body, s.maxAgeSecs
	s.mu.Unlock()

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "max-age="+itoa(maxAge))

	if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
