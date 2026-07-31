package service

import (
	"context"
	"testing"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	dcstodcsdb "digital-contracting-service/internal/dcstodcs/db"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
)

type staticCEKRepo struct {
	records []artifactstore.CEKRecord
}

func (s *staticCEKRepo) Fetch(context.Context, artifactstore.Scope, string) (*artifactstore.CEKRecord, error) {
	return nil, nil
}

func (s *staticCEKRepo) List(context.Context, artifactstore.Scope) ([]artifactstore.CEKRecord, error) {
	return s.records, nil
}

func (s *staticCEKRepo) Insert(context.Context, artifactstore.Scope, string, []byte) (bool, error) {
	return false, nil
}

func (s *staticCEKRepo) Shred(context.Context, artifactstore.Scope, string, string) (int64, error) {
	return 0, nil
}

type staticEraseRepo struct {
	rows []dcstodcsdb.EraseRequest
}

func (s *staticEraseRepo) Upsert(context.Context, string, string) error        { return nil }
func (s *staticEraseRepo) RecordAttempt(context.Context, string, string) error { return nil }
func (s *staticEraseRepo) Confirm(context.Context, string, string) error       { return nil }
func (s *staticEraseRepo) GetPending(context.Context) ([]dcstodcsdb.EraseRequest, error) {
	return nil, nil
}
func (s *staticEraseRepo) ListByDID(context.Context, string) ([]dcstodcsdb.EraseRequest, error) {
	return s.rows, nil
}

func erasureStatusService(keys *staticCEKRepo, erases *staticEraseRepo) *contractStorageArchivesrvc {
	return &contractStorageArchivesrvc{Keys: keys, ERepo: erases}
}

const statusTestIRI = "did:web:local.example:contract:status"

func TestErasureStatusLiveWithoutRecords(t *testing.T) {
	svc := erasureStatusService(&staticCEKRepo{}, &staticEraseRepo{})
	res, err := svc.ErasureStatus(context.Background(), &contractstoragearchive.ErasureStatusPayload{Did: statusTestIRI})
	if err != nil {
		t.Fatalf("ErasureStatus: %v", err)
	}
	if res.LocalStatus != "live" || res.ShreddedAt != nil || len(res.Peers) != 0 {
		t.Fatalf("expected a live status without peers, got %+v", res)
	}
}

func TestErasureStatusShreddedSurfacesDestructionRecord(t *testing.T) {
	shreddedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	shreddedBy := "archive-manager@local"
	reason := "Art. 17 request"
	svc := erasureStatusService(&staticCEKRepo{records: []artifactstore.CEKRecord{
		{ScopeKind: "contract", ScopeID: statusTestIRI, RecipientDID: "did:web:local.example",
			ShreddedAt: &shreddedAt, ShreddedBy: &shreddedBy, ShredReason: &reason},
	}}, &staticEraseRepo{})

	res, err := svc.ErasureStatus(context.Background(), &contractstoragearchive.ErasureStatusPayload{Did: statusTestIRI})
	if err != nil {
		t.Fatalf("ErasureStatus: %v", err)
	}
	if res.LocalStatus != "shredded" {
		t.Fatalf("expected shredded, got %q", res.LocalStatus)
	}
	if res.ShreddedAt == nil || *res.ShreddedAt != "2026-07-28T12:00:00Z" {
		t.Fatalf("unexpected shredded_at: %v", res.ShreddedAt)
	}
	if res.ShreddedBy == nil || *res.ShreddedBy != shreddedBy || res.ShredReason == nil || *res.ShredReason != reason {
		t.Fatalf("destruction record incomplete: %+v", res)
	}
}

func TestErasureStatusMapsPendingAndConfirmedPeers(t *testing.T) {
	requested := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	tried := requested.Add(5 * time.Minute)
	confirmed := requested.Add(10 * time.Minute)
	svc := erasureStatusService(&staticCEKRepo{}, &staticEraseRepo{rows: []dcstodcsdb.EraseRequest{
		{DID: statusTestIRI, PeerDID: "did:web:pending.example", RequestedAt: requested, RetryCount: 3, LastTriedAt: &tried},
		{DID: statusTestIRI, PeerDID: "did:web:confirmed.example", RequestedAt: requested, ConfirmedAt: &confirmed},
	}})

	res, err := svc.ErasureStatus(context.Background(), &contractstoragearchive.ErasureStatusPayload{Did: statusTestIRI})
	if err != nil {
		t.Fatalf("ErasureStatus: %v", err)
	}
	if len(res.Peers) != 2 {
		t.Fatalf("expected two peer entries, got %d", len(res.Peers))
	}
	pending, confirmedPeer := res.Peers[0], res.Peers[1]
	if pending.Status != "pending" || pending.RetryCount != 3 || pending.LastTriedAt == nil || pending.ConfirmedAt != nil {
		t.Fatalf("unexpected pending state: %+v", pending)
	}
	if confirmedPeer.Status != "confirmed" || confirmedPeer.ConfirmedAt == nil || *confirmedPeer.ConfirmedAt != "2026-07-28T12:10:00Z" {
		t.Fatalf("unexpected confirmed state: %+v", confirmedPeer)
	}
}
