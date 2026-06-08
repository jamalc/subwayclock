// Command serve is the main server for the subway clock application.
//
// It loads GTFS data, starts a realtime poller for the MTA feeds, and serves
// arrival information over HTTP.
//
// Usage:
//
//	go run ./cmd/serve [-config path/to/config.yaml]
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jamalc/subwayclock/internal/arrivals"
	"github.com/jamalc/subwayclock/internal/gtfs"
	"github.com/jamalc/subwayclock/internal/realtime"
	"github.com/jamalc/subwayclock/internal/server"

	"gopkg.in/yaml.v3"
)

var (
	configPath = flag.String("config", "cmd/serve/config.yaml", "path to config file")
)

type config struct {
	// MTAAPIKey is the API key for the MTA realtime data API.
	// See https://api.mta.info/.
	MTAAPIKey string `yaml:"mta_api_key"`

	// RealtimeInterval is the number of seconds between MTA API polls.
	RealtimeInterval int `yaml:"realtime_interval"`

	// GTFSSubway is the path to the GTFS subway feed zip file.
	GTFSSubway string `yaml:"gtfs_subway"`

	// GTFSSupplemented is the path to the GTFS supplemented feed zip, which
	// adds data such as trip updates.
	GTFSSupplemented string `yaml:"gtfs_supplemented"`
}

// load reads the config from the specified YAML file path.
func (c *config) load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return yaml.NewDecoder(f).Decode(c)
}

func main() {
	flag.Parse()

	cfg := config{}
	if err := cfg.load(*configPath); err != nil {
		log.Fatal("config:", err)
	}

	static := gtfs.NewStaticData()
	log.Println("Loading GTFS data...")
	if err := static.Load(cfg.GTFSSubway, cfg.GTFSSupplemented); err != nil {
		log.Fatal("GTFS:", err)
	}
	log.Println("GTFS loaded")

	log.Println("Starting realtime poller...")
	poller := realtime.NewPoller(cfg.MTAAPIKey)
	if err := poller.Start(time.Duration(cfg.RealtimeInterval) * time.Second); err != nil {
		log.Fatal("realtime poller:", err)
	}
	log.Println("Realtime poller ready")

	engine := arrivals.NewEngine(static, poller)

	mux := http.NewServeMux()
	server.NewHandler(engine, static).RegisterRoutes(mux)

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe("0.0.0.0:8080", mux))
}
