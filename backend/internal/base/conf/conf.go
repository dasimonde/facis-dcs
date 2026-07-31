// Package conf centralizes timing/topic configuration shared across domains
// (transaction timeouts, outbox/cron polling intervals, the event-bus topic
// name), so these values are changed in one place rather than duplicated
// per domain.
package conf

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func TransactionTimeout() time.Duration {
	return 1 * time.Minute
}

// SystemToken is the in-cluster service credential the background PDF
// regenerator presents to the internal signing primitives (it runs on NATS
// events with no user JWT). Empty when unset — no system caller is accepted.
func SystemToken() string {
	return os.Getenv("DCS_SYSTEM_TOKEN")
}

func HTTPClientTimeout() time.Duration {
	return 1 * time.Minute
}

func OutboxProcessorTimeOut() time.Duration {
	return 1 * time.Second
}

// OutboxPublishTimeOut is the poll interval for republishing outbox events
// on NATS (see event.OutboxProcessor.startPublishingJob): much tighter than
// OutboxProcessorTimeOut because publishing is a cheap, single NATS call per
// event, unlike the TSA/IPFS round-trips the (slower) anchoring loop does.
func OutboxPublishTimeOut() time.Duration {
	return 100 * time.Millisecond
}

func EventBusTopic() string {
	return "dcs"
}

// AuditCheckpointTimestampRetry is how often checkpoints that were anchored
// while the TSA was unreachable are retried. Roots are immutable, so attaching
// the timestamp later costs nothing but this delay.
func AuditCheckpointTimestampRetry() time.Duration {
	return 30 * time.Second
}

// OutboxAnchorMaxAttempts is how often an audit event is retried before it is
// dead-lettered. Generous, because most anchoring failures are transient (the
// TSA or the IPFS store being briefly unavailable) and dead-lettering an event
// means it never enters the tamper-evident trail.
func OutboxAnchorMaxAttempts() int {
	return 50
}

// AuditCheckpointReadLimit bounds how many checkpoints a full-trail read walks
// back through, so the read stays within a request deadline once the log has
// real history.
func AuditCheckpointReadLimit() int {
	return 500
}

func LoginAttemptsThresholdInDuration() int {
	return 5
}

func LoginLockoutDuration() time.Duration {
	return 15 * time.Minute
}

// SyncFailCronJobTimeOut is how often the DB-backed sync-fail scheduler
// re-attempts contract PDF ships that were dropped or failed. Because it reads
// its work from the sync_fails table rather than the event bus, it is the
// reliable delivery backbone for DCS-to-DCS federation — independent of NATS
// at-most-once event redelivery — so it must reconcile well within a
// negotiation round, not once a day. DCS_SYNC_FAIL_RETRY_INTERVAL (a Go
// duration, e.g. "15s") overrides the default; BDD/e2e set it low so
// replication is deterministic within the test wait.
func SyncFailCronJobTimeOut() time.Duration {
	if v := os.Getenv("DCS_SYNC_FAIL_RETRY_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 5 * time.Minute
}

// PDFRegenerationRetryTimeOut is how often the background regenerator
// re-attempts entities left without a stored PDF. Lifecycle events arrive over
// NATS at-most-once and a regeneration that fails is not redelivered, so a
// single transient pdf-core or IPFS failure otherwise leaves the artifact
// missing permanently: the contract cannot be exported, and a cross-instance
// contract cannot be shipped at all — the sync-fail scheduler keeps retrying a
// ship that has nothing to send. This is that path's counterpart, so it shares
// its cadence. DCS_PDF_REGENERATION_RETRY_INTERVAL (a Go duration, e.g. "10s")
// overrides the default; BDD/e2e set it low.
func PDFRegenerationRetryTimeOut() time.Duration {
	if v := os.Getenv("DCS_PDF_REGENERATION_RETRY_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return 5 * time.Minute
}

// ArchiveExpiringWindow is how far ahead the archive dashboard's
// expiring-contracts list looks (DCS-FR-CSA-04, DCS-FR-CSA-21).
// DCS_ARCHIVE_EXPIRING_WINDOW_DAYS (a positive integer number of days)
// overrides the 30-day default; any non-positive or unparsable value keeps
// the default, like the other env-overridable accessors in this package.
func ArchiveExpiringWindow() time.Duration {
	if v := strings.TrimSpace(os.Getenv("DCS_ARCHIVE_EXPIRING_WINDOW_DAYS")); v != "" {
		if days, err := strconv.Atoi(v); err == nil && days > 0 {
			return time.Duration(days) * 24 * time.Hour
		}
	}
	return 30 * 24 * time.Hour
}

// APIRateLimitPerMinute is the per-credential request budget for
// authenticated API interactions (DCS-FR-CWE-28). DCS_API_RATE_LIMIT_PER_MINUTE
// (a positive integer) overrides the default; any non-positive or unparsable
// value keeps the default, like the other env-overridable accessors here.
func APIRateLimitPerMinute() int {
	if v := strings.TrimSpace(os.Getenv("DCS_API_RATE_LIMIT_PER_MINUTE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 3000
}

// AESCertNameMatchRequired reports whether the sole-control gate (ADR-20)
// enforces cert-subject to PID name matching for an AES-level signature. It
// is mandatory and non-configurable for QES (eIDAS Annex I requires a
// qualified certificate to carry the signatory's verified name); for AES it
// defaults to enforced too — closing exactly the shared/self-signed-key hole
// the gate exists for — but an operator can relax it with
// DCS_AES_CERT_NAME_MATCH_REQUIRED=false for a deployment whose AES ceremony
// legitimately cannot bind a certificate name (documented operator choice,
// never the default).
func AESCertNameMatchRequired() bool {
	v := strings.TrimSpace(os.Getenv("DCS_AES_CERT_NAME_MATCH_REQUIRED"))
	if v == "" {
		return true
	}
	enabled, err := strconv.ParseBool(v)
	if err != nil {
		return true
	}
	return enabled
}
