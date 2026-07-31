package base

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
)

type softwareKeyAgreement struct{ priv *ecdsa.PrivateKey }

func (a softwareKeyAgreement) DeriveECDH(peerPub *ecdsa.PublicKey) ([]byte, error) {
	priv, err := a.priv.ECDH()
	if err != nil {
		return nil, err
	}
	pub, err := peerPub.ECDH()
	if err != nil {
		return nil, err
	}
	return priv.ECDH(pub)
}

type memoryCEKRepo struct{ records []*artifactstore.CEKRecord }

func (m *memoryCEKRepo) Fetch(_ context.Context, scope artifactstore.Scope, recipientDID string) (*artifactstore.CEKRecord, error) {
	var shredded *artifactstore.CEKRecord
	for _, r := range m.records {
		if r.ScopeKind != string(scope.Kind) || r.ScopeID != scope.ID || r.RecipientDID != recipientDID {
			continue
		}
		if r.ShreddedAt == nil {
			return r, nil
		}
		shredded = r
	}
	return shredded, nil
}

func (m *memoryCEKRepo) List(_ context.Context, scope artifactstore.Scope) ([]artifactstore.CEKRecord, error) {
	var records []artifactstore.CEKRecord
	for _, r := range m.records {
		if r.ScopeKind == string(scope.Kind) && r.ScopeID == scope.ID {
			records = append(records, *r)
		}
	}
	return records, nil
}

func (m *memoryCEKRepo) Insert(_ context.Context, scope artifactstore.Scope, recipientDID string, wrappedCEK []byte) (bool, error) {
	m.records = append(m.records, &artifactstore.CEKRecord{
		ScopeKind: string(scope.Kind), ScopeID: scope.ID, RecipientDID: recipientDID, WrappedCEK: wrappedCEK,
	})
	return true, nil
}

func (m *memoryCEKRepo) Shred(_ context.Context, scope artifactstore.Scope, shreddedBy, reason string) (int64, error) {
	now := time.Now().UTC()
	var n int64
	for _, r := range m.records {
		if r.ScopeKind == string(scope.Kind) && r.ScopeID == scope.ID && r.ShreddedAt == nil {
			r.ShreddedAt = &now
			r.ShreddedBy = &shreddedBy
			r.ShredReason = &reason
			n++
		}
	}
	return n, nil
}

func newTestArtifactStore(t *testing.T) (*artifactstore.Store, *memoryCEKRepo) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	repo := &memoryCEKRepo{}
	return artifactstore.New(nil, repo, softwareKeyAgreement{priv: priv}, "did:web:test.local", "did:web:test.local#dcs-ecdh", &priv.PublicKey), repo
}

// storedEntryBytes builds the stored form of an audit entry the way
// OutboxProcessor.writeEntry does: plaintext header, encrypted event_data.
func storedEntryBytes(t *testing.T, store *artifactstore.Store, scope artifactstore.Scope, plaintextBody []byte) []byte {
	t.Helper()
	body, err := store.Encrypt(context.Background(), scope, plaintextBody)
	if err != nil {
		t.Fatalf("encrypt body: %v", err)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encode body: %v", err)
	}
	did := scope.ID
	entry := datatype.AuditLogEntry{
		ID:           42,
		Component:    "CONTRACT_WORKFLOW_ENGINE",
		EventType:    "CREATE_CONTRACT",
		EventData:    encoded,
		DID:          &did,
		CreatedAt:    time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC),
		CEKScopeKind: string(scope.Kind),
		CEKScopeID:   scope.ID,
		Nonce:        "00112233445566778899aabbccddeeff",
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal audit log entry: %v", err)
	}
	return raw
}

func TestOpenAuditLogEntryDecryptsBody(t *testing.T) {
	store, _ := newTestArtifactStore(t)
	scope := artifactstore.ContractScope("did:example:contract:1")
	raw := storedEntryBytes(t, store, scope, []byte(`{"created_by":"alice"}`))

	reader := AuditTrailReader{Artifacts: store}
	got, err := reader.openAuditLogEntry(context.Background(), raw)
	if err != nil {
		t.Fatalf("openAuditLogEntry: %v", err)
	}
	if got.ID != 42 || got.Component != "CONTRACT_WORKFLOW_ENGINE" || got.EventType != "CREATE_CONTRACT" {
		t.Fatalf("header mismatch: %+v", got)
	}
	if string(got.EventData) != `{"created_by":"alice"}` {
		t.Fatalf("body not decrypted: %s", got.EventData)
	}
}

func TestOpenAuditLogEntryStoredFormIsOpaque(t *testing.T) {
	store, _ := newTestArtifactStore(t)
	scope := artifactstore.ContractScope("did:example:contract:2")
	raw := storedEntryBytes(t, store, scope, []byte(`{"created_by":"alice"}`))

	var stored datatype.AuditLogEntry
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("unmarshal stored entry: %v", err)
	}
	if string(stored.EventData) == `{"created_by":"alice"}` {
		t.Fatal("stored event_data is plaintext")
	}
	// Header fields stay plaintext.
	if stored.Component != "CONTRACT_WORKFLOW_ENGINE" || stored.DID == nil {
		t.Fatalf("header not plaintext: %+v", stored)
	}
}

func TestOpenAuditLogEntryAfterShredYieldsErasedBody(t *testing.T) {
	store, repo := newTestArtifactStore(t)
	scope := artifactstore.ContractScope("did:example:contract:erase")
	raw := storedEntryBytes(t, store, scope, []byte(`{"created_by":"alice"}`))

	if _, err := repo.Shred(context.Background(), scope, "admin", "Art. 17"); err != nil {
		t.Fatalf("shred: %v", err)
	}
	store.Forget(scope)

	reader := AuditTrailReader{Artifacts: store}
	got, err := reader.openAuditLogEntry(context.Background(), raw)
	if err != nil {
		t.Fatalf("openAuditLogEntry after shred must not fail: %v", err)
	}
	if !got.IsErased() {
		t.Fatalf("body must be the defined erased marker, got %s", got.EventData)
	}
	if got.Component != "CONTRACT_WORKFLOW_ENGINE" || got.DID == nil {
		t.Fatalf("header must survive the shred: %+v", got)
	}
}

// TestLeafHashStableAcrossShred pins the erasure/tamper-evidence coexistence:
// the leaf hash is computed over the stored bytes, which a shred never touches,
// so checkpoint inclusion proofs keep verifying after the body is erased.
func TestLeafHashStableAcrossShred(t *testing.T) {
	store, repo := newTestArtifactStore(t)
	scope := artifactstore.ContractScope("did:example:contract:proof")
	raw := storedEntryBytes(t, store, scope, []byte(`{"created_by":"alice"}`))

	before := MerkleLeafHash(raw)
	if _, err := repo.Shred(context.Background(), scope, "admin", "Art. 17"); err != nil {
		t.Fatalf("shred: %v", err)
	}
	store.Forget(scope)
	if after := MerkleLeafHash(raw); after != before {
		t.Fatalf("leaf hash changed across shred: %s != %s", after, before)
	}
}

func TestOpenAuditLogEntryWithoutScopeKeepsBody(t *testing.T) {
	entry := datatype.AuditLogEntry{ID: 7, Component: "SYSTEM", EventType: "X", EventData: json.RawMessage(`{"k":"v"}`)}
	raw, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	reader := AuditTrailReader{}
	got, err := reader.openAuditLogEntry(context.Background(), raw)
	if err != nil {
		t.Fatalf("openAuditLogEntry: %v", err)
	}
	if string(got.EventData) != `{"k":"v"}` {
		t.Fatalf("scope-less body must pass through: %s", got.EventData)
	}
}
