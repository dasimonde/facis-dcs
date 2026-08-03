package artifactstore

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"digital-contracting-service/internal/base/ipfs"
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

// fakeObjectStore is a content-addressed in-memory IPFS stand-in.
type fakeObjectStore struct {
	blobs map[string][]byte
}

func newFakeObjectStore() *fakeObjectStore { return &fakeObjectStore{blobs: map[string][]byte{}} }

func (f *fakeObjectStore) CreateFile(_ context.Context, data any) (*ipfs.IPFSResult, error) {
	raw, ok := data.([]byte)
	if !ok {
		return nil, fmt.Errorf("fake object store only accepts []byte, got %T", data)
	}
	sum := sha256.Sum256(raw)
	cid := hex.EncodeToString(sum[:])
	f.blobs[cid] = append([]byte(nil), raw...)
	result := &ipfs.IPFSResult{Data: raw}
	result.Identifier.Format = "CID"
	result.Identifier.Value = cid
	return result, nil
}

func (f *fakeObjectStore) FetchFile(cid string) (*ipfs.IPFSResult, error) {
	raw, ok := f.blobs[cid]
	if !ok {
		return nil, fmt.Errorf("cid %s not found", cid)
	}
	result := &ipfs.IPFSResult{Data: raw}
	result.Identifier.Format = "CID"
	result.Identifier.Value = cid
	return result, nil
}

// memoryCEKRepo mirrors the Postgres semantics incl. the live-unique index.
type memoryCEKRepo struct {
	records []*CEKRecord
}

func (m *memoryCEKRepo) Fetch(_ context.Context, scope Scope, recipientDID string) (*CEKRecord, error) {
	var shredded *CEKRecord
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

func (m *memoryCEKRepo) List(_ context.Context, scope Scope) ([]CEKRecord, error) {
	var records []CEKRecord
	for _, r := range m.records {
		if r.ScopeKind == string(scope.Kind) && r.ScopeID == scope.ID {
			records = append(records, *r)
		}
	}
	return records, nil
}

func (m *memoryCEKRepo) Insert(_ context.Context, scope Scope, recipientDID string, wrappedCEK []byte) (bool, error) {
	for _, r := range m.records {
		// A shredded scope never accepts new keys (mirrors the advisory-lock
		// + tombstone check of the Postgres repo).
		if r.ScopeKind == string(scope.Kind) && r.ScopeID == scope.ID && r.ShreddedAt != nil {
			return false, nil
		}
	}
	for _, r := range m.records {
		if r.ScopeKind == string(scope.Kind) && r.ScopeID == scope.ID && r.RecipientDID == recipientDID && r.ShreddedAt == nil {
			return false, nil
		}
	}
	m.records = append(m.records, &CEKRecord{
		ScopeKind:    string(scope.Kind),
		ScopeID:      scope.ID,
		RecipientDID: recipientDID,
		WrappedCEK:   append([]byte(nil), wrappedCEK...),
		CreatedAt:    time.Now().UTC(),
	})
	return true, nil
}

func (m *memoryCEKRepo) Shred(_ context.Context, scope Scope, shreddedBy, reason string) (int64, error) {
	var affected int64
	now := time.Now().UTC()
	for _, r := range m.records {
		if r.ScopeKind == string(scope.Kind) && r.ScopeID == scope.ID && r.ShreddedAt == nil {
			r.ShreddedAt = &now
			r.ShreddedBy = &shreddedBy
			r.ShredReason = &reason
			affected++
		}
	}
	return affected, nil
}

func newTestStore(t *testing.T) (*Store, *fakeObjectStore, *memoryCEKRepo) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	objects := newFakeObjectStore()
	repo := &memoryCEKRepo{}
	store := New(objects, repo, softwareKeyAgreement{priv: priv}, "did:web:test.local", "did:web:test.local#dcs-ecdh", &priv.PublicKey)
	return store, objects, repo
}

func TestPutGetRoundtrip(t *testing.T) {
	store, objects, repo := newTestStore(t)
	ctx := context.Background()
	scope := ContractScope("did:web:test.local:contract:1")
	pdf := []byte("%PDF-1.7\nthe contract content")

	cid, err := store.Put(ctx, scope, pdf)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(ctx, scope, cid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(got, pdf) {
		t.Fatal("Get did not return the byte-identical plaintext")
	}

	stored := objects.blobs[cid]
	if bytes.HasPrefix(stored, []byte("%PDF")) || bytes.Contains(stored, []byte("contract content")) {
		t.Fatal("stored blob is not opaque at rest")
	}

	record, err := repo.Fetch(ctx, scope, "did:web:test.local")
	if err != nil || record == nil {
		t.Fatalf("wrapped cek record missing: %v", err)
	}
	if bytes.Contains(record.WrappedCEK, []byte("contract content")) {
		t.Fatal("wrapped cek record leaks content")
	}
}

func TestGetAfterCacheEvictionUnwrapsPersistedCEK(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()
	scope := ContractScope("did:web:test.local:contract:2")

	cid, err := store.Put(ctx, scope, []byte("payload"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	store.Forget(scope)

	got, err := store.Get(ctx, scope, cid)
	if err != nil {
		t.Fatalf("Get after cache drop: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("got %q, want payload", got)
	}
}

func TestScopeIsolation(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()
	scopeA := ContractScope("did:web:test.local:contract:a")
	scopeB := ContractScope("did:web:test.local:contract:b")

	cid, err := store.Put(ctx, scopeA, []byte("scope-a secret"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	// Ensure scope B has its own CEK, then try reading A's blob under B.
	if _, err := store.Put(ctx, scopeB, []byte("scope-b content")); err != nil {
		t.Fatalf("Put scope b: %v", err)
	}
	if _, err := store.Get(ctx, scopeB, cid); err == nil {
		t.Fatal("scope B must not decrypt scope A's artifact")
	}
}

func TestShreddedScopeReturnsDefinedError(t *testing.T) {
	store, _, repo := newTestStore(t)
	ctx := context.Background()
	scope := ContractScope("did:web:test.local:contract:erase-me")

	cid, err := store.Put(ctx, scope, []byte("to be erased"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	affected, err := repo.Shred(ctx, scope, "did:web:test.local", "Art. 17 request")
	if err != nil || affected != 1 {
		t.Fatalf("Shred affected=%d err=%v", affected, err)
	}
	store.Forget(scope)

	if _, err := store.Get(ctx, scope, cid); !IsShredded(err) {
		t.Fatalf("Get after shred: got %v, want ShreddedError", err)
	}
	if _, err := store.Put(ctx, scope, []byte("resurrection")); !IsShredded(err) {
		t.Fatalf("Put after shred: got %v, want ShreddedError", err)
	}
	if _, err := store.Decrypt(ctx, scope, []byte("blob")); !IsShredded(err) {
		t.Fatalf("Decrypt after shred: got %v, want ShreddedError", err)
	}
}

func TestGetWithoutKeyFails(t *testing.T) {
	store, _, _ := newTestStore(t)
	if _, err := store.Get(context.Background(), ContractScope("did:web:test.local:contract:none"), "no-cid"); err == nil {
		t.Fatal("Get without a CEK record must fail")
	}
}

func TestEncryptDecryptValueRoundtrip(t *testing.T) {
	store, _, _ := newTestStore(t)
	ctx := context.Background()
	scope := store.InstanceScope()

	blob, err := store.Encrypt(ctx, scope, []byte(`{"actor":"alice"}`))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Contains(blob, []byte("alice")) {
		t.Fatal("encrypted value leaks plaintext")
	}
	got, err := store.Decrypt(ctx, scope, blob)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != `{"actor":"alice"}` {
		t.Fatalf("roundtrip mismatch: %s", got)
	}
}
