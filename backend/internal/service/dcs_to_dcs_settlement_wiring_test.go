package service

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// The verification rules next door are only worth what the handler does with
// them: a PostSettlement that stored the artifact and verified it afterwards —
// or that took the peer's word for the document it settled — would leave every
// one of those tests passing while a tampered artifact became the evidence a
// signature is gated on. Both are pinned here.
//
// This is the counterpart of the signing gate's own wiring test
// (signingmanagement/command/apply_settlementgate_test.go): a check that
// nothing reaches refuses nothing.
func TestPostSettlementStoresNothingItHasNotVerified(t *testing.T) {
	body := settlementHandlerBody(t, "PostSettlement")

	verify := callSites(body, "verifyShippedSettlement")
	upsert := callSites(body, "UpsertSettlement")
	if len(verify) != 1 {
		t.Fatalf("PostSettlement calls verifyShippedSettlement %d times, want exactly once", len(verify))
	}
	if len(upsert) != 1 {
		t.Fatalf("PostSettlement calls UpsertSettlement %d times, want exactly once: a second write is a path around the verification", len(upsert))
	}
	if upsert[0] < verify[0] {
		t.Error("the settlement is stored before it is verified: an artifact that does not verify is already evidence")
	}

	// Transport authentication and the ADR-19 federation gate are what the PDF
	// ship applies before it accepts anything; a settlement deposits evidence a
	// signature hangs on, so it cannot apply less.
	for _, gate := range []string{"VerifyPeerChallenge", "Check"} {
		sites := callSites(body, gate)
		if len(sites) == 0 {
			t.Errorf("PostSettlement no longer calls %s: any host can deposit counterparty evidence", gate)
			continue
		}
		if sites[0] > upsert[0] {
			t.Errorf("%s runs after the settlement is stored", gate)
		}
	}

	// The document the peer is held to is digested from THIS instance's own
	// copy. Reading it off the shipped artifact instead would make the version
	// binding self-certifying, and the whole gate vacuous.
	digest := callSites(body, "ContractDocumentDigest")
	if len(digest) == 0 {
		t.Fatal("PostSettlement no longer digests the contract document it holds: the version binding is whatever the peer claims")
	}
	if digest[0] > verify[0] {
		t.Error("the local document digest is computed after verification, so it cannot be what the artifact was checked against")
	}
}

// settlementHandlerBody returns the body of a top-level or method declaration
// in the settlement handler file.
func settlementHandlerBody(t *testing.T, name string) *ast.BlockStmt {
	t.Helper()
	const file = "dcs_to_dcs_settlement.go"
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name && fn.Body != nil {
			return fn.Body
		}
	}
	t.Fatalf("%s declares no %s", file, name)
	return nil
}

// callSites returns the positions of every call to name in body, in source
// order.
func callSites(body *ast.BlockStmt, name string) []token.Pos {
	var positions []token.Pos
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var called string
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			called = fun.Name
		case *ast.SelectorExpr:
			called = fun.Sel.Name
		}
		if called == name {
			positions = append(positions, call.Pos())
		}
		return true
	})
	return positions
}

// A withdrawal DELETES the evidence a signature is gated on, so the ordering
// matters more here than anywhere else on this channel: everything that decides
// whether the deletion is legitimate has to have run first.
func TestPostSettlementWithdrawalDeletesNothingItHasNotVerified(t *testing.T) {
	body := settlementHandlerBody(t, "PostSettlementWithdrawal")

	del := callSites(body, "DeleteSettlementsBy")
	if len(del) != 1 {
		t.Fatalf("PostSettlementWithdrawal calls DeleteSettlementsBy %d times, want exactly once", len(del))
	}
	for _, gate := range []string{"VerifyPeerChallenge", "Check", "verifyShippedWithdrawal", "withdrawalTakesBack"} {
		sites := callSites(body, gate)
		if len(sites) == 0 {
			t.Errorf("PostSettlementWithdrawal no longer calls %s: any host could strip the evidence a signature is gated on", gate)
			continue
		}
		if sites[0] > del[0] {
			t.Errorf("%s runs after the settlement is deleted", gate)
		}
	}
}

// The rejected design, pinned. Auto-withdrawing a settlement when the document
// it names is no longer the one held would delete the counterparty's evidence
// exactly when it is needed: the first signature seals odrl:Offer into
// odrl:Agreement, so on the receive path a digest mismatch is the NORMAL state
// of a peer that has signed, and the instance that has to countersign would
// have destroyed what lets it. A settlement goes only when the party that made
// it says so, over the withdrawal channel.
func TestNoSettlementIsDroppedByTheReceivePath(t *testing.T) {
	for _, handler := range []string{"PostSettlement", "verifyShippedSettlement", "supersededDocumentDigests"} {
		if sites := callSites(settlementHandlerBody(t, handler), "DeleteSettlementsBy"); len(sites) > 0 {
			t.Errorf("%s deletes a settlement: a digest mismatch on receive is not evidence that a peer withdrew", handler)
		}
	}
}
