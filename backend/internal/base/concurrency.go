package base

import "errors"

// ErrUpdatedElsewhere marks an optimistic-concurrency refusal (ADR-0007): the
// caller's view of a resource is older than what is stored, so its command was
// not applied.
//
// Nothing failed and the caller is not wrong — a background writer (the PDF
// regenerator, an arriving peer ship) advances updated_at between a client's
// read and its command. Re-reading and reissuing succeeds. Every refusal is
// built by wrapping this, so callers can tell the case apart instead of
// matching on message text, and the API can answer with a conflict a client may
// retry rather than an internal error, which claims the request will never
// succeed.
var ErrUpdatedElsewhere = errors.New("was updated elsewhere")
