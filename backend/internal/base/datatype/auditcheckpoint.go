package datatype

import "time"

// AuditCheckpoint commits to every audit entry anchored in one processing tick.
// The Merkle root fixes both the membership and the order of the batch, and
// PrevRoot chains it to the preceding checkpoint, so the sequence of roots is
// an append-only log: an entry can neither be edited nor removed nor reordered
// without changing a root that has already been timestamped.
//
// It replaces the per-entry global chain, which forced every event through a
// strictly sequential TSA+IPFS round-trip and let one failing event stall the
// whole trail. Order inside a checkpoint is the outbox order; order across
// checkpoints is the root chain. A retried event is anchored in a later
// checkpoint while carrying its own CreatedAt — the log then states separately
// when the event happened and by when its existence was proven, which is all a
// timestamp can attest anyway.
//
// The TSA signature is deliberately NOT part of these bytes: the root is
// immutable once written, so a checkpoint whose timestamp is still pending can
// be timestamped later (see OutboxProcessor.startTimestampingJob) without
// rewriting anything. That keeps a TSA outage from blocking the audit trail.
type AuditCheckpoint struct {
	Seq        int64     `json:"seq"`
	Root       string    `json:"root"`
	PrevRoot   *string   `json:"prev_root"`
	LeafHashes []string  `json:"leaf_hashes"`
	LeafCIDs   []string  `json:"leaf_cids"`
	CreatedAt  time.Time `json:"created_at"`
}

// AuditCheckpointRecord is the database index over the checkpoints: the trail
// itself lives in IPFS, this is how the head, the pending timestamps and the
// walk order are found.
type AuditCheckpointRecord struct {
	Seq           int64      `db:"seq"`
	CID           string     `db:"cid"`
	Root          string     `db:"root"`
	PrevRoot      *string    `db:"prev_root"`
	LeafCount     int        `db:"leaf_count"`
	TsaSignature  *string    `db:"tsa_signature"`
	CreatedAt     time.Time  `db:"created_at"`
	TimestampedAt *time.Time `db:"timestamped_at"`
}
