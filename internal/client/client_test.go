package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchArrivals_FlattensNested(t *testing.T) {
	body := `[
		{"stop_id":"A27N","stop_name":"Franklin Av","groups":[
			{"route":"A","color":"0039A6","route_name":"8 Avenue","headsign":"Inwood","etas":["3m","9m"]},
			{"route":"C","color":"0039A6","route_name":"8 Avenue Local","headsign":"Northbound","etas":[]},
			{"route":"F","color":"FF6319","route_name":"Queens Blvd","headsign":"Coney Is","etas":["2m"]}
		]}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()

	host := strings.TrimPrefix(srv.URL, "http://")
	parts := strings.Split(host, ":")
	port := 0
	if len(parts) == 2 {
		port = atoiTest(t, parts[1])
	}
	c := NewClient(parts[0], port)

	groups, err := c.FetchArrivals([]string{"A27N"})
	if err != nil {
		t.Fatalf("FetchArrivals: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("got %d flattened groups, want 3", len(groups))
	}
	byRoute := map[string]int{}
	for i, g := range groups {
		byRoute[g.Route] = i
	}
	if g := groups[byRoute["C"]]; len(g.ETAs) != 0 {
		t.Errorf("C should be a no-service row (empty etas), got %v", g.ETAs)
	}
	if groups[byRoute["A"]].StopName != "Franklin Av" {
		t.Errorf("stop name not attached: %+v", groups[byRoute["A"]])
	}
}

func atoiTest(t *testing.T, s string) int {
	t.Helper()
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
