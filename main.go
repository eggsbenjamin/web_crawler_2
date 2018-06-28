package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/eggsbenjamin/web_crawler/crawler"
)

func main() {
	workersStr := mustGetEnv("WORKERS")
	workers, err := strconv.Atoi(workersStr)
	if err != nil {
		log.Fatalf("env var 'WORKERS' is non-numeric: %s", workersStr)
	}
	if workers == 0 {
		log.Fatalf("env var 'WORKERS' must be greater than zero: %d", workers)
	}

	url := mustGetEnv("URL")
	c := crawler.New(workers, &http.Client{Timeout: time.Second * 2})

	if err := c.Crawl(url, os.Stdout); err != nil {
		log.Fatalf("error crawling %s: %q", url, err)
	}
}

func mustGetEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("env var '%s' not set", k)
	}
	return v
}
