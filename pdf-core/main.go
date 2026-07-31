package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strings"

)

// loadEnvFile reads KEY=VALUE pairs from path and sets them in the environment.
// Lines starting with # and blank lines are ignored.
// Missing file is silently ignored.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k := strings.TrimSpace(key)
		if existing, alreadySet := os.LookupEnv(k); alreadySet && existing != "" {
			continue
		}
		if err := os.Setenv(k, strings.TrimSpace(value)); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func main() {
	if err := loadEnvFile(".env"); err != nil {
		log.Fatalf("load .env: %v", err)
	}

	addr := os.Getenv("DCS_PDF_CORE_ADDR")
	if addr == "" {
		addr = "0.0.0.0:8080"
	}
	server := &http.Server{
		Addr:    addr,
		Handler: newServer(),
	}

	log.Printf("DCS-PDF-CORE listening on %s", server.Addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
