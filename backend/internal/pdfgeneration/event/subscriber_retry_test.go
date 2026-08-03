package event

import (
	"context"
	"errors"
	"testing"
	"time"

	"digital-contracting-service/internal/pdfgeneration/provenance"
)

// The retry pass has no event to work from — the one that first asked for the
// regeneration was consumed and is not redelivered. It must therefore drive the
// handler off the DID alone, with an effective time of now, so the handler
// re-reads state and content from the record.
func TestRetryOneDrivesRegenerationFromTheDIDAlone(t *testing.T) {
	var seen minimalCWEEvent
	before := time.Now().UTC()

	s := &Subscriber{}
	s.retryOne("contract", "did:contract:1", func(_ context.Context, evt minimalCWEEvent) error {
		seen = evt
		return nil
	})

	if seen.DID != "did:contract:1" {
		t.Fatalf("the regenerated DID must be the one retried, got %q", seen.DID)
	}
	if seen.Reason != "" || seen.NewState != "" {
		t.Fatalf("a retry carries no event fields, got reason %q / state %q", seen.Reason, seen.NewState)
	}
	if seen.OccurredAt.Before(before) {
		t.Fatalf("the attempt must be effective now, got %s", seen.OccurredAt)
	}
}

// A retry that fails must not take the pass down with it: the next entity still
// gets its attempt, and the failed one is picked up again on the next tick
// because its record still shows no stored PDF.
func TestRetryOneSurvivesAFailedRegeneration(t *testing.T) {
	s := &Subscriber{}

	s.retryOne("template", "did:template:1", func(context.Context, minimalCWEEvent) error {
		return errors.New("artifact store unavailable")
	})
}

// The retry sweep selects a contract on its record alone, and the regeneration
// path's answer to a missing artifact is a FRESH render. For a contract past
// signing — whose stored CID went missing because the peer-receive transaction
// rolled back after the row committed — that render is an UNSIGNED PDF, issued
// a new lifecycle VC and stamped as authoritative over the counterparty's
// provenance, every sweep until it succeeds. A frozen contract must never reach
// the fresh-render branch.
func TestAFrozenContractWithoutAStoredPDFIsNeverFreshRendered(t *testing.T) {
	frozen, err := frozenArtifactVerdict("did:contract:1", "SIGNED", "", "", "active", signed)
	if err == nil {
		t.Fatal("a signed contract with no stored PDF must refuse regeneration, not fall through to a fresh render")
	}
	if frozen {
		t.Fatal("the refusal must be reported as an error, not as nothing-to-do")
	}

	// The same contract WITH its signed artifact is simply left alone.
	frozen, err = frozenArtifactVerdict("did:contract:1", "SIGNED", "bafy...", "active", "active", signed)
	if err != nil || !frozen {
		t.Fatalf("a stored frozen artifact must be left untouched, got frozen=%t err=%v", frozen, err)
	}

	// A pre-signing contract still regenerates, with or without an artifact.
	for _, storedCID := range []string{"", "bafy..."} {
		frozen, err = frozenArtifactVerdict("did:contract:1", "DRAFT", storedCID, "draft", "draft", unsigned)
		if err != nil || frozen {
			t.Fatalf("a draft contract must still regenerate (cid %q), got frozen=%t err=%v", storedCID, frozen, err)
		}
	}
}

var (
	signed   = func() (bool, error) { return true, nil }
	unsigned = func() (bool, error) { return false, nil }
)

// A contract reaches a frozen C2PA state without ever being signed: the BDD
// terminate path is DRAFT -> APPROVED -> terminate, and the expiry cron flips
// unsigned contracts to EXPIRED. Freezing on the target state alone declines
// the FIRST render into that state, so pdf_state never catches up with the
// contract and ExportContractPdf — which serves only when the two agree —
// polls to its deadline and answers "being regenerated" permanently.
func TestAnUnsignedContractReachingAFrozenStateIsStillRendered(t *testing.T) {
	for state, c2paState := range map[string]string{"TERMINATED": "terminated", "EXPIRED": "expired"} {
		frozen, err := frozenArtifactVerdict("did:contract:1", state, "bafy...", "draft", c2paState, unsigned)
		if err != nil {
			t.Fatalf("%s: an unsigned contract has nothing to refuse over: %v", state, err)
		}
		if frozen {
			t.Fatalf("%s: an unsigned contract must render its first %q artifact, not be frozen out of it", state, c2paState)
		}
	}
}

// The counterpart the freeze exists for: the same transition on a contract that
// IS signed leaves the signed bytes alone.
func TestASignedContractReachingAFrozenStateIsNeverRendered(t *testing.T) {
	frozen, err := frozenArtifactVerdict("did:contract:1", "TERMINATED", "bafy...", "draft", "terminated", signed)
	if err != nil || !frozen {
		t.Fatalf("a signed contract must never be re-rendered, got frozen=%t err=%v", frozen, err)
	}
}

// The signature lookup is a DB query; a failure must stop the regeneration
// rather than be read as "not signed" and fresh-render a signed contract.
func TestAFailedSignatureLookupStopsTheRegeneration(t *testing.T) {
	frozen, err := frozenArtifactVerdict("did:contract:1", "SIGNED", "bafy...", "draft", "active",
		func() (bool, error) { return false, errors.New("database unavailable") })
	if err == nil {
		t.Fatal("an unanswerable signature lookup must propagate, not decide the verdict")
	}
	if frozen {
		t.Fatal("the refusal must be reported as an error, not as nothing-to-do")
	}
}

// 25 permanently unrenderable rows would otherwise fill the batch on every
// tick forever and starve every recoverable failure behind them. A failing
// entity backs off, and after its attempts are spent it drops out of the work
// list entirely.
func TestRetryBudgetBacksOffAndGivesUp(t *testing.T) {
	budget := &retryBudget{}
	budget.pace(time.Minute)
	now := time.Now()

	if !budget.ready("contract", "did:contract:1", now) {
		t.Fatal("an entity with no failure history is attempted immediately")
	}
	budget.failed("contract", "did:contract:1", now)
	if budget.ready("contract", "did:contract:1", now.Add(30*time.Second)) {
		t.Fatal("a failed entity must wait out its backoff instead of being retried on the next tick")
	}
	if !budget.ready("contract", "did:contract:1", now.Add(2*time.Minute)) {
		t.Fatal("the entity must be attempted again once its backoff has elapsed")
	}

	for attempt := 1; attempt < regenerationRetryAttempts; attempt++ {
		budget.failed("contract", "did:contract:1", now)
	}
	if budget.ready("contract", "did:contract:1", now.Add(24*time.Hour)) {
		t.Fatal("an entity that has spent its attempts must not be attempted again")
	}
	if got := budget.exhausted("contract"); len(got) != 1 || got[0] != "did:contract:1" {
		t.Fatalf("the exhausted entity must be excluded from the work-list query, got %v", got)
	}
	if got := budget.exhausted("template"); len(got) != 0 {
		t.Fatalf("templates share no budget with contracts, got %v", got)
	}

	// A success clears the history: the next failure starts from a fresh budget.
	budget.succeeded("contract", "did:contract:1")
	if !budget.ready("contract", "did:contract:1", now) || len(budget.exhausted("contract")) != 0 {
		t.Fatal("a successful regeneration must clear the entity's budget")
	}
}

// The regeneration context carries a deadline, so a wedged pdf-core or artifact
// store cannot hold the regenerator open indefinitely.
func TestRegenerationContextIsBounded(t *testing.T) {
	s := &Subscriber{}
	ctx, cancel := s.regenerationContext()
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("a regeneration attempt must run under a deadline")
	}
	if remaining := time.Until(deadline); remaining > regenerationTimeout {
		t.Fatalf("the deadline must be at most %s away, got %s", regenerationTimeout, remaining)
	}
}

// A peer's signature exists only in the peer's database: CountSignedSignatures
// reads THIS instance's contract_signatures, and the receive path writes no row
// there. So carriesSignature is false for a counterparty's signed artifact, and
// a local terminate (allowed from OFFERED/NEGOTIATION/APPROVED) or the expiry
// cron would otherwise append a C2PA manifest onto its signed bytes. What
// freezes it is the state receivepdf records for the stored artifact, read from
// the shipped PDF itself.
func TestAPeerSignedArtifactIsFrozenWithoutALocalSignatureRow(t *testing.T) {
	peerSigned := []byte("%PDF-1.7\n/Type /Sig /ByteRange [0 840 960 1200]\n")
	stored, err := provenance.ArtifactC2PAState("OFFERED", peerSigned)
	if err != nil {
		t.Fatalf("map the received artifact's state: %v", err)
	}

	for _, target := range []string{"terminated", "expired", "draft"} {
		frozen, err := frozenArtifactVerdict("did:contract:1", "OFFERED", "bafy...", stored, target, unsigned)
		if err != nil {
			t.Fatalf("target %s: %v", target, err)
		}
		if !frozen {
			t.Fatalf("target %s: a counterparty's signed artifact must never be re-rendered", target)
		}
	}
}
