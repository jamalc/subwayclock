// Command fetch is a utility to download the GTFS feed files.
//
// The feed files are large and updated infrequently, so they aren't included
// in the repository. Instead, this command can be run to download them to a
// local directory, and then the server can be pointed at that directory with
// the config's GTFSSubway and GTFSSupplemented fields.
//
// Usage:
//
//	go run ./cmd/fetch [-dir path/to/feeds] [-regular-max-age duration] [-supplemented-max-age duration]
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	regularURL      = "https://rrgtfsfeeds.s3.amazonaws.com/gtfs_subway.zip"
	supplementedURL = "https://rrgtfsfeeds.s3.amazonaws.com/gtfs_supplemented.zip"
)

func main() {
	dir := flag.String("dir", ".", "directory to store zip files")
	regularMaxAge := flag.Duration("regular-max-age", 24*time.Hour, "skip regular feed if younger than this")
	suppMaxAge := flag.Duration("supplemented-max-age", 1*time.Hour, "skip supplemented feed if younger than this")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0755); err != nil {
		log.Fatal(err)
	}

	fetch(filepath.Join(*dir, "gtfs_subway.zip"), regularURL, *regularMaxAge)
	fetch(filepath.Join(*dir, "gtfs_supplemented.zip"), supplementedURL, *suppMaxAge)
}

// fetch downloads url to path, unless path already exists and is younger than
// maxAge.
func fetch(path, url string, maxAge time.Duration) {
	info, err := os.Stat(path)
	if err == nil && time.Since(info.ModTime()) < maxAge {
		fmt.Printf("%s is fresh (age %s), skipping\n", filepath.Base(path), time.Since(info.ModTime()).Round(time.Second))
		return
	}

	fmt.Printf("downloading %s...\n", filepath.Base(path))
	if err := download(url, path); err != nil {
		log.Fatalf("failed: %v", err)
	}
	fmt.Printf("saved %s\n", path)
}

// download fetches url and saves it to path, writing to a temp file and
// renaming so a failure can't leave a partial file behind.
func download(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
