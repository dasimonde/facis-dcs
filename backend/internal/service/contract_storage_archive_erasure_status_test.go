package service

import (
	"testing"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	dcstodcsdb "digital-contracting-service/internal/dcstodcs/db"
)

const statusTestIRI = "did:web:local.example:contract:status"

func TestErasureStatusLiveWithoutRecords(t *testing.T) {
	res := erasureStatusResponse(statusTestIRI, nil, nil)
	if res.LocalStatus != "live" || res.ShreddedAt != nil || len(res.Peers) != 0 {
		t.Fatalf("expected a live status without peers, got %+v", res)
	}
}

func TestErasureStatusShreddedSurfacesDestructionRecord(t *testing.T) {
	shreddedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	shreddedBy := "archive-manager@local"
	reason := "Art. 17 request"
	records := []artifactstore.CEKRecord{
		{ScopeKind: "contract", ScopeID: statusTestIRI, RecipientDID: "did:web:local.example",
			ShreddedAt: &shreddedAt, ShreddedBy: &shreddedBy, ShredReason: &reason},
	}

	res := erasureStatusResponse(statusTestIRI, records, nil)
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
	requests := []dcstodcsdb.EraseRequest{
		{DID: statusTestIRI, PeerDID: "did:web:pending.example", RequestedAt: requested, RetryCount: 3, LastTriedAt: &tried},
		{DID: statusTestIRI, PeerDID: "did:web:confirmed.example", RequestedAt: requested, ConfirmedAt: &confirmed},
	}

	res := erasureStatusResponse(statusTestIRI, nil, requests)
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

// An erase that the peer refused for good shows as refused, with the peer's
// answer, so the Archive Manager sees that the peer's copy stays.
func TestErasureStatusShowsARefusedPeer(t *testing.T) {
	requested := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	answer := "erase rejected: this instance holds no copy of the contract shared with the requesting peer"
	res := erasureStatusResponse(statusTestIRI, nil, []dcstodcsdb.EraseRequest{
		{DID: statusTestIRI, PeerDID: "did:web:refused.example", RequestedAt: requested, LastTriedAt: &requested, RefusedAt: &requested, LastError: &answer},
	})
	if len(res.Peers) != 1 || res.Peers[0].Status != "refused" || res.Peers[0].LastError == nil || *res.Peers[0].LastError != answer {
		t.Fatalf("unexpected refused state: %+v", res.Peers)
	}
}
