// Package realtime polls the MTA GTFS-realtime feeds and caches the latest trip
// updates for the arrivals engine. Server-side, native only (used by the serve
// binary).
package realtime

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/MobilityData/gtfs-realtime-bindings/golang/gtfs"
	"google.golang.org/protobuf/proto"
)

// httpClient has an explicit timeout so a hung MTA connection can't stall a
// fetch goroutine indefinitely.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// feedURLs maps a label to each subway feed endpoint.
// The 7 train is included in the 1234567S feed, not a separate endpoint.
var feedURLs = map[string]string{
	"1234567S": "https://api-endpoint.mta.info/Dataservice/mtagtfsfeeds/nyct%2Fgtfs",
	"ACE":      "https://api-endpoint.mta.info/Dataservice/mtagtfsfeeds/nyct%2Fgtfs-ace",
	"BDFM":     "https://api-endpoint.mta.info/Dataservice/mtagtfsfeeds/nyct%2Fgtfs-bdfm",
	"G":        "https://api-endpoint.mta.info/Dataservice/mtagtfsfeeds/nyct%2Fgtfs-g",
	"JZ":       "https://api-endpoint.mta.info/Dataservice/mtagtfsfeeds/nyct%2Fgtfs-jz",
	"NQRW":     "https://api-endpoint.mta.info/Dataservice/mtagtfsfeeds/nyct%2Fgtfs-nqrw",
	"L":        "https://api-endpoint.mta.info/Dataservice/mtagtfsfeeds/nyct%2Fgtfs-l",
}

// Poller periodically fetches the GTFS-realtime feeds and caches the latest
// data in memory for the arrivals engine to query.
type Poller struct {
	apiKey string
	mu     sync.RWMutex
	feeds  map[string]*gtfs.FeedMessage
}

// NewPoller creates a new Poller with the given MTA API key.
func NewPoller(apiKey string) *Poller {
	return &Poller{
		apiKey: apiKey,
		feeds:  make(map[string]*gtfs.FeedMessage),
	}
}

// Start does an initial fetch and returns an error if every feed failed.
// A partial success (some feeds ok, some not) is logged but not fatal —
// the poller will retry on the next tick.
func (p *Poller) Start(interval time.Duration) error {
	if err := p.fetchAll(); err != nil {
		return err
	}
	go func() {
		for range time.Tick(interval) {
			if err := p.fetchAll(); err != nil {
				log.Printf("realtime poll failed: %v", err)
			}
		}
	}()
	return nil
}

// fetchAll fetches all feeds in parallel. Returns an error only if every
// single feed failed — individual failures are logged and the previous
// cached value is kept.
func (p *Poller) fetchAll() error {
	type result struct {
		key string
		msg *gtfs.FeedMessage
		err error
	}

	ch := make(chan result, len(feedURLs))
	for key, url := range feedURLs {
		go func(key, url string) {
			msg, err := p.fetch(url)
			ch <- result{key, msg, err}
		}(key, url)
	}

	var successCount int
	for range feedURLs {
		r := <-ch
		if r.err != nil {
			log.Printf("feed %s: %v", r.key, r.err)
			continue
		}
		p.mu.Lock()
		p.feeds[r.key] = r.msg
		p.mu.Unlock()
		successCount++
	}

	if successCount == 0 {
		return fmt.Errorf("all %d realtime feeds failed", len(feedURLs))
	}
	return nil
}

// fetch retrieves and parses the GTFS-realtime feed at url, setting the
// required API key header.
func (p *Poller) fetch(url string) (*gtfs.FeedMessage, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", p.apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	msg := &gtfs.FeedMessage{}
	return msg, proto.Unmarshal(b, msg)
}

// AllEntities returns every entity across all cached feeds.
func (p *Poller) AllEntities() []*gtfs.FeedEntity {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var all []*gtfs.FeedEntity
	for _, msg := range p.feeds {
		all = append(all, msg.Entity...)
	}
	return all
}
