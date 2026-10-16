package collector

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type cacheEntry struct {
	etag       string
	body       []byte
	validUntil time.Time
}

// CachedClient is a rate-limit-aware, ETag-caching HTTP client for one upstream source.
// "Rate-limit-aware" means every request is gated by a per-source TokenBucket and a 429
// response is retried with bounded exponential backoff, never abandoned outright within
// MaxRetries. "ETag-caching" means a response inside its advertised Cache-Control max-age
// window is served from the local cache with no network call at all, and a response past that
// window is re-validated with If-None-Match before assuming it changed.
type CachedClient struct {
	HTTP        *http.Client
	Bucket      *TokenBucket
	MaxRetries  int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry

	// Metrics, for the "caching cut upstream calls X%" measurement.
	UpstreamCalls int
	CacheHits     int // served from cache, no network call
	NotModified   int // network call made, got 304
	Fetched       int // network call made, got 200
	Dropped       int // exhausted retries without a usable response
}

func NewCachedClient(bucket *TokenBucket) *CachedClient {
	return &CachedClient{
		HTTP:        &http.Client{Timeout: 5 * time.Second},
		Bucket:      bucket,
		MaxRetries:  12,
		BaseBackoff: 20 * time.Millisecond,
		MaxBackoff:  2 * time.Second,
		cache:       make(map[string]cacheEntry),
	}
}

// Get returns the body for url, honoring the local cache window, and reports whether a real
// network call was made.
func (c *CachedClient) Get(url string) (body []byte, calledUpstream bool, err error) {
	c.mu.Lock()
	entry, ok := c.cache[url]
	c.mu.Unlock()

	if ok && time.Now().Before(entry.validUntil) {
		c.mu.Lock()
		c.CacheHits++
		c.mu.Unlock()
		return entry.body, false, nil
	}

	backoff := c.BaseBackoff
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if !c.Bucket.Wait(c.MaxBackoff * 4) {
			// Could not get a token within a generous window; back off and retry rather than
			// give up, since 0 dropped records is the point of bounded backoff existing.
			time.Sleep(backoff)
			backoff = nextBackoff(backoff, c.MaxBackoff)
			continue
		}

		req, reqErr := http.NewRequest(http.MethodGet, url, nil)
		if reqErr != nil {
			return nil, false, reqErr
		}
		if ok && entry.etag != "" {
			req.Header.Set("If-None-Match", entry.etag)
		}

		resp, doErr := c.HTTP.Do(req)
		c.mu.Lock()
		c.UpstreamCalls++
		c.mu.Unlock()

		if doErr != nil {
			time.Sleep(backoff)
			backoff = nextBackoff(backoff, c.MaxBackoff)
			continue
		}

		switch resp.StatusCode {
		case http.StatusTooManyRequests, http.StatusServiceUnavailable:
			wait := backoff
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, convErr := strconv.Atoi(ra); convErr == nil {
					wait = time.Duration(secs) * time.Second
				}
			}
			resp.Body.Close()
			time.Sleep(wait)
			backoff = nextBackoff(backoff, c.MaxBackoff)
			continue

		case http.StatusNotModified:
			resp.Body.Close()
			c.mu.Lock()
			c.NotModified++
			entry.validUntil = time.Now().Add(cacheWindow(resp))
			c.cache[url] = entry
			cached := entry.body
			c.mu.Unlock()
			return cached, true, nil

		case http.StatusOK:
			data, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				return nil, true, readErr
			}
			newEntry := cacheEntry{
				etag:       resp.Header.Get("ETag"),
				body:       data,
				validUntil: time.Now().Add(cacheWindow(resp)),
			}
			c.mu.Lock()
			c.Fetched++
			c.cache[url] = newEntry
			c.mu.Unlock()
			return data, true, nil

		default:
			resp.Body.Close()
			return nil, true, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
		}
	}

	c.mu.Lock()
	c.Dropped++
	c.mu.Unlock()
	return nil, true, fmt.Errorf("exhausted %d retries against %s", c.MaxRetries, url)
}

func nextBackoff(cur, max time.Duration) time.Duration {
	next := cur * 2
	if next > max {
		return max
	}
	return next
}

// cacheWindow reads Cache-Control: max-age=N from the response, defaulting to a short window
// so a source with no caching headers still benefits from request coalescing within a poll tick.
func cacheWindow(resp *http.Response) time.Duration {
	cc := resp.Header.Get("Cache-Control")
	if cc == "" {
		return 200 * time.Millisecond
	}
	var maxAge int
	if _, err := fmt.Sscanf(cc, "max-age=%d", &maxAge); err == nil {
		return time.Duration(maxAge) * time.Second
	}
	return 200 * time.Millisecond
}
