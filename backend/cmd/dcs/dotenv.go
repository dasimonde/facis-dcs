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
