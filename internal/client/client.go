// Package client fetches arrival data from the serve backend over HTTP.
//
// Used by the clock loop on both the native simulator and the on-device
// (TinyGo) firmware, so it must stay TinyGo-compatible.
package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/jamalc/subwayclock/internal/api"
)

// httpClient has an explicit timeout so a hung connection can't stall the
// poll loop.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// Client is the API client for fetching arrival data from the server.
type Client struct {
	host string
	port int
}

// NewClient creates a new Client with the given server host and port.
func NewClient(host string, port int) *Client {
	return &Client{host: host, port: port}
}

// FetchArrivals fetches arrivals for the given stop IDs and returns them as a
// flat list of groups, sorted by stop name, then route, then headsign.
func (c *Client) FetchArrivals(stops []string) ([]api.FlatArrivalGroup, error) {
	var u url.URL
	u.Scheme = "http"
	u.Host = c.host + ":" + strconv.Itoa(c.port)
	u.Path = "/arrivals"
	q := u.Query()
	for _, stop := range stops {
		q.Add("stop", stop)
	}
	u.RawQuery = q.Encode()

	resp, err := httpClient.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}

	var stopArrivals []api.StopArrivals
	if err := json.NewDecoder(resp.Body).Decode(&stopArrivals); err != nil {
		return nil, err
	}

	var groups []api.FlatArrivalGroup
	for _, s := range stopArrivals {
		for _, g := range s.Groups {
			group := api.FlatArrivalGroup{ArrivalGroup: g}
			group.StopID = s.StopID
			group.StopName = s.StopName
			groups = append(groups, group)
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].StopID != groups[j].StopID {
			return groups[i].StopName < groups[j].StopName
		}
		if groups[i].Route != groups[j].Route {
			return groups[i].Route < groups[j].Route
		}
		return groups[i].Headsign < groups[j].Headsign
	})
	return groups, nil
}
