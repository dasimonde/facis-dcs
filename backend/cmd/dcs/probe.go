package main

import (
	"fmt"
	"net/http"
	"time"
)

// probeHTTP performs a single GET to the given URL and returns an error if the
// response is not 2xx. Used at startup to fail fast when a required dependency
// is down.
func probeHTTP(rawURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return err
	}
	err = resp.Body.Close()
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// probeHTTPUntilReady polls probe until it succeeds or timeout elapses, so a
// required dependency that is merely slow to start (common under CI resource
// pressure) does not crash-loop the DCS. It still returns the last error — a
// hard fail — if the dependency never becomes reachable within the window.
func probeHTTPUntilReady(timeout time.Duration, probe func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		err := probe()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return err
		}
		time.Sleep(2 * time.Second)
	}
}
