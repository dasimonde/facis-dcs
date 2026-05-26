package fcschemas

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
)

const (
	defaultSyncRetryInterval = 10 * time.Second
	defaultSyncMaxWait       = 10 * time.Minute
)

// SyncWithRetry runs Sync until success or max wait elapses.
// Retries on transient FC/network errors (e.g. FC pod not ready yet).
//
// Env (optional):
//   - FC_SCHEMA_SYNC_RETRY_INTERVAL — pause between attempts (default 10s)
//   - FC_SCHEMA_SYNC_MAX_WAIT — give up after this duration (default 10m; 0 = single attempt)
func SyncWithRetry(ctx context.Context, fc *fcclient.FederatedCatalogueClient) error {
	interval := durationEnv("FC_SCHEMA_SYNC_RETRY_INTERVAL", defaultSyncRetryInterval)
	maxWait := durationEnv("FC_SCHEMA_SYNC_MAX_WAIT", defaultSyncMaxWait)

	if maxWait <= 0 {
		return Sync(ctx, fc)
	}

	deadline := time.Now().Add(maxWait)
	attempt := 0
	var lastErr error

	for {
		attempt++
		lastErr = Sync(ctx, fc)
		if lastErr == nil {
			if attempt > 1 {
				log.Printf("fc schema sync succeeded after %d attempts", attempt)
			}
			return nil
		}
		if !isRetryableSyncError(lastErr) {
			return lastErr
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("fc schema sync gave up after %s (%d attempts): %w", maxWait, attempt, lastErr)
		}

		log.Printf("fc schema sync attempt %d failed, retry in %s: %v", attempt, interval, lastErr)
		select {
		case <-ctx.Done():
			return fmt.Errorf("fc schema sync cancelled: %w", lastErr)
		case <-time.After(interval):
		}
	}
}

// durationEnv reads a time.Duration from the given environment variable, falling back to the default if not set or invalid.
func durationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Printf("fc schema sync: invalid %s=%q, using %s", key, raw, fallback)
		return fallback
	}
	return d
}

// isRetryableSyncError determines whether the given error is a transient FC/network error that should trigger a retry.
func isRetryableSyncError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	msg := strings.ToLower(err.Error())
	for _, fragment := range []string{
		"request failed",
		"connection reset by peer",
	} {
		if strings.Contains(msg, fragment) {
			return true
		}
	}
	return false
}
