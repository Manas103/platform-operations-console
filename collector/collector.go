package collector

import (
	"encoding/json"
	"fmt"
)

// SourceEndpoint pairs a source with the URL its client polls.
type SourceEndpoint struct {
	Source Source
	URL    string
}

// Collector owns one CachedClient per source (so one source's rate limit or backoff never
// blocks another) and normalizes every fetched record into the shared Store.
type Collector struct {
	Store     *Store
	Clients   map[Source]*CachedClient
	Endpoints []SourceEndpoint
}

func NewCollector(store *Store, endpoints []SourceEndpoint, bucketCapacity, bucketRefillPerSec float64) *Collector {
	clients := make(map[Source]*CachedClient)
	for _, ep := range endpoints {
		clients[ep.Source] = NewCachedClient(NewTokenBucket(bucketCapacity, bucketRefillPerSec))
	}
	return &Collector{Store: store, Clients: clients, Endpoints: endpoints}
}

// PollOnce fetches every source once and upserts whatever it got into the store. Returns the
// number of records normalized and whether each source's fetch resulted in a real network call.
func (c *Collector) PollOnce() (recordsWritten int, calledUpstream map[Source]bool, err error) {
	calledUpstream = make(map[Source]bool)
	for _, ep := range c.Endpoints {
		client := c.Clients[ep.Source]
		body, called, getErr := client.Get(ep.URL)
		calledUpstream[ep.Source] = called
		if getErr != nil {
			return recordsWritten, calledUpstream, fmt.Errorf("source %s: %w", ep.Source, getErr)
		}
		var records []Record
		if err := json.Unmarshal(body, &records); err != nil {
			return recordsWritten, calledUpstream, fmt.Errorf("source %s: decoding: %w", ep.Source, err)
		}
		for _, r := range records {
			if err := c.Store.Upsert(ep.Source, r); err != nil {
				return recordsWritten, calledUpstream, err
			}
			recordsWritten++
		}
	}
	return recordsWritten, calledUpstream, nil
}

// TotalUpstreamCalls and TotalCacheHits aggregate every client's counters, for the "caching
// cut upstream calls X%" measurement.
func (c *Collector) TotalUpstreamCalls() int {
	total := 0
	for _, client := range c.Clients {
		total += client.UpstreamCalls
	}
	return total
}

func (c *Collector) TotalCacheHits() int {
	total := 0
	for _, client := range c.Clients {
		total += client.CacheHits
	}
	return total
}

func (c *Collector) TotalDropped() int {
	total := 0
	for _, client := range c.Clients {
		total += client.Dropped
	}
	return total
}
