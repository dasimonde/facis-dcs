package request

import (
	"context"
	"time"
)

// PublicKeyCache provides an interface for caching public keys.
type PublicKeyCache interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
}
