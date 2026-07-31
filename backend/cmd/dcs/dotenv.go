package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// loadDotenvIfPresent loads .env from the current working directory when present.
// Existing environment variables are preserved.
func loadDotenvIfPresent() error {
	const dotenvPath = ".env"

	if _, err := os.Stat(dotenvPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to stat %s: %w", dotenvPath, err)
	}

	if err := godotenv.Load(dotenvPath); err != nil {
		return fmt.Errorf("failed to load %s: %w", dotenvPath, err)
	}

	return nil
}

func loadDotenvFile(dotenvFile string) error {
	if _, err := os.Stat(dotenvFile); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to stat %s: %w", dotenvFile, err)
	}

	if err := godotenv.Load(dotenvFile); err != nil {
		return fmt.Errorf("failed to load %s: %w", dotenvFile, err)
	}

	return nil
}
