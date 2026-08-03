package dcstodcs

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"digital-contracting-service/internal/base/identity"
	db2 "digital-contracting-service/internal/dcstodcs/db"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
)

// eraserTestDIDDocument builds a DIDDocument for the local instance via the
// same NewDIDDocument path production uses.
func eraserTestDIDDocument(t *testing.T, host string) *identity.DIDDocument {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// The x5c the counterparty validates the authenticating key's chain against.
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	didJSON := map[string]any{
		"id":             "did:web:" + host,
		"authentication": []any{"#key-1"},
		"verificationMethod": []map[string]any{
			{
				"id": "did:web:" + host + "#key-1",
				"publicKeyJwk": map[string]any{
					"kty": "EC",
					"crv": "P-256",
					"x":   base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
					"y":   base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
					"x5c": []string{base64.StdEncoding.EncodeToString(certDER)},
				},
			},
		},
	}
	raw, err := json.Marshal(didJSON)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "did.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := identity.NewDIDDocument(path, key)
	if err != nil {
		t.Fatalf("NewDIDDocument: %v", err)
	}
	return doc
}

type shredCall struct{ iri, actor, reason string }

type fakeShredder struct {
	calls    []shredCall
	affected int64
	err      error
}

func (f *fakeShredder) Shred(_ context.Context, iri, actor, reason string) (int64, error) {
	f.calls = append(f.calls, shredCall{iri: iri, actor: actor, reason: reason})
	return f.affected, f.err
}

type fakeParties struct {
	parties []string
	err     error
}

func (f *fakeParties) Parties(context.Context, string) ([]string, error) { return f.parties, f.err }

type fakeEraseRepo struct {
	rows map[string]*db2.EraseRequest
}

func newFakeEraseRepo() *fakeEraseRepo { return &fakeEraseRepo{rows: map[string]*db2.EraseRequest{}} }

func (f *fakeEraseRepo) key(did, peer string) string { return did + "|" + peer }

func (f *fakeEraseRepo) Upsert(_ context.Context, did, peer string) error {
	if _, ok := f.rows[f.key(did, peer)]; ok {
		return nil
	}
	f.rows[f.key(did, peer)] = &db2.EraseRequest{DID: did, PeerDID: peer, RequestedAt: time.Now().UTC()}
	return nil
}

func (f *fakeEraseRepo) RecordAttempt(_ context.Context, did, peer string) error {
	row := f.rows[f.key(did, peer)]
	now := time.Now().UTC()
	row.RetryCount++
	row.LastTriedAt = &now
	return nil
}

func (f *fakeEraseRepo) Confirm(_ context.Context, did, peer string) error {
	row := f.rows[f.key(did, peer)]
	if row.ConfirmedAt == nil {
		now := time.Now().UTC()
		row.ConfirmedAt = &now
	}
	return nil
}

func (f *fakeEraseRepo) GetPending(context.Context) ([]db2.EraseRequest, error) {
	var pending []db2.EraseRequest
	for _, row := range f.rows {
		if row.ConfirmedAt == nil {
			pending = append(pending, *row)
		}
	}
	return pending, nil
}

func (f *fakeEraseRepo) ListByDID(_ context.Context, did string) ([]db2.EraseRequest, error) {
	var rows []db2.EraseRequest
	for _, row := range f.rows {
		if row.DID == did {
			rows = append(rows, *row)
		}
	}
	return rows, nil
}

type fakePeerSender struct {
	fail  bool
	calls []*dcstodcs.DCSToDCSContractEraseRequest
}

func (f *fakePeerSender) SendErase(_ context.Context, _ string, req *dcstodcs.DCSToDCSContractEraseRequest) error {
	f.calls = append(f.calls, req)
	if f.fail {
		return errors.New("peer unreachable")
	}
	return nil
}

func newTestEraser(t *testing.T, parties []string, sender *fakePeerSender) (*ContractEraser, *fakeShredder, *fakeEraseRepo) {
	t.Helper()
	doc := eraserTestDIDDocument(t, "local.example")
	shredder := &fakeShredder{affected: 2}
	repo := newFakeEraseRepo()
	return &ContractEraser{
		DIDDocument: *doc,
		Shredder:    shredder,
		Parties:     &fakeParties{parties: parties},
		ERepo:       repo,
		Sender:      sender,
	}, shredder, repo
}

const eraserTestIRI = "did:web:local.example:contract:1"

func TestEraseContractShredsLocallyAndConfirmsPeer(t *testing.T) {
	sender := &fakePeerSender{}
	eraser, shredder, repo := newTestEraser(t, []string{"did:web:local.example", "did:web:peer.example"}, sender)

	if err := eraser.EraseContract(context.Background(), eraserTestIRI, "archive-manager@local", "Art. 17 request"); err != nil {
		t.Fatalf("EraseContract: %v", err)
	}

	if len(shredder.calls) != 1 || shredder.calls[0].actor != "archive-manager@local" || shredder.calls[0].reason != "Art. 17 request" {
		t.Fatalf("unexpected shred calls: %+v", shredder.calls)
	}
	if len(sender.calls) != 1 {
		t.Fatalf("expected one peer erase request, got %d", len(sender.calls))
	}
	req := sender.calls[0]
	if req.FromPeerDid != "did:web:local.example" || req.ContractIri != eraserTestIRI {
		t.Fatalf("unexpected erase request: %+v", req)
	}
	// The challenge is signed with the key this instance publishes for
	// authenticating itself — exactly what the receiving peer checks.
	if err := eraser.DIDDocument.VerifyPeerChallenge(nil, []byte(req.SecretValue), req.SecretHash); err != nil {
		t.Fatalf("erase request challenge does not verify: %v", err)
	}
	row := repo.rows[repo.key(eraserTestIRI, "did:web:peer.example")]
	if row == nil || row.ConfirmedAt == nil {
		t.Fatalf("peer erase not confirmed: %+v", row)
	}
}

func TestEraseContractLocalOnlyContractShredsWithoutPeerTraffic(t *testing.T) {
	sender := &fakePeerSender{}
	eraser, shredder, repo := newTestEraser(t, []string{"did:web:local.example"}, sender)

	if err := eraser.EraseContract(context.Background(), eraserTestIRI, "actor", "reason"); err != nil {
		t.Fatalf("EraseContract: %v", err)
	}
	if len(shredder.calls) != 1 {
		t.Fatalf("expected the local shred, got %+v", shredder.calls)
	}
	if len(sender.calls) != 0 || len(repo.rows) != 0 {
		t.Fatal("a purely local contract must not trigger peer traffic")
	}
}

func TestEraseContractDoesNotShredWhenPartiesUnreadable(t *testing.T) {
	sender := &fakePeerSender{}
	eraser, shredder, _ := newTestEraser(t, nil, sender)
	eraser.Parties = &fakeParties{err: errors.New("contract unreadable")}

	if err := eraser.EraseContract(context.Background(), eraserTestIRI, "actor", "reason"); err == nil {
		t.Fatal("expected the party-resolution error to propagate")
	}
	if len(shredder.calls) != 0 {
		t.Fatal("nothing may be destroyed when the contract cannot be read")
	}
}

func TestEraseContractPeerDownLeavesPendingRowAndRetryDelivers(t *testing.T) {
	sender := &fakePeerSender{fail: true}
	eraser, _, repo := newTestEraser(t, []string{"did:web:local.example", "did:web:peer.example"}, sender)

	if err := eraser.EraseContract(context.Background(), eraserTestIRI, "actor", "reason"); err != nil {
		t.Fatalf("EraseContract with peer down must not fail: %v", err)
	}
	row := repo.rows[repo.key(eraserTestIRI, "did:web:peer.example")]
	if row == nil || row.ConfirmedAt != nil {
		t.Fatalf("expected a pending erase row, got %+v", row)
	}
	if row.RetryCount != 1 || row.LastTriedAt == nil {
		t.Fatalf("expected the failed attempt recorded, got %+v", row)
	}

	// Peer back up: the retry scheduler delivers and confirms.
	sender.fail = false
	eraser.RetryPending(context.Background())
	if row.ConfirmedAt == nil {
		t.Fatalf("expected the retry to confirm the erase, got %+v", row)
	}
	if len(sender.calls) != 2 {
		t.Fatalf("expected exactly two delivery attempts, got %d", len(sender.calls))
	}

	// Confirmed rows are not retried again.
	eraser.RetryPending(context.Background())
	if len(sender.calls) != 2 {
		t.Fatal("a confirmed erase must not be re-delivered")
	}
}
