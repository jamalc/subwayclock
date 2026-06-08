// Package api defines the data types for the HTTP API responses.
//
// They are shared by both the server and the client.
package api

// ArrivalGroup is a group of arrivals for one route and headsign at a stop.
type ArrivalGroup struct {
	Route     string   `json:"route"`
	Color     string   `json:"color"`
	RouteName string   `json:"route_name"`
	Headsign  string   `json:"headsign"`
	ETAs      []string `json:"etas"`
}

// FlatArrivalGroup is an ArrivalGroup with the stop ID and name flattened in.
type FlatArrivalGroup struct {
	ArrivalGroup
	StopID   string `json:"stop_id"`
	StopName string `json:"stop_name"`
}

// StopArrivals is the set of arrival groups served at one stop.
type StopArrivals struct {
	StopID   string         `json:"stop_id"`
	StopName string         `json:"stop_name"`
	Groups   []ArrivalGroup `json:"groups"`
}

// ErrorResponse is the JSON body returned for API errors.
type ErrorResponse struct {
	Message string `json:"message"`
}
