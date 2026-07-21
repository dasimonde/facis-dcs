// Package conf centralizes timing/topic configuration shared across domains
// (transaction timeouts, outbox/cron polling intervals, the event-bus topic
// name), so these values are changed in one place rather than duplicated
// per domain.
package conf

import (
	"os"
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
