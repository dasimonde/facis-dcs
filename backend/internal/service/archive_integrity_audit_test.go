package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

func TestArchiveTSATimestampComparisonUsesRFC3161SecondPrecision(t *testing.T) {
	storedAt := time.Date(2026, 7, 30, 21, 31, 43, 900_000_000, time.UTC)

	if archiveTSATimestampPrecedesStoredAt(
		time.Date(2026, 7, 30, 21, 31, 43, 0, time.UTC),
		storedAt,
	) {
		t.Fatal("a TSA timestamp in the same whole second must not precede storedAt")
	}
	if !archiveTSATimestampPrecedesStoredAt(
		time.Date(2026, 7, 30, 21, 31, 42, 0, time.UTC),
		storedAt,
	) {
		t.Fatal("a TSA timestamp from the previous second must precede storedAt")
	}
}

func TestParseArchiveNotaryChainAcceptsValidChain(t *testing.T) {
	first := archiveNotaryEvent{
		EventType:       "ARCHIVE_STORED",
		DID:             "did:example:contract:1",
		ContractVersion: 1,
		ArchiveEntryID:  "did:example:contract:1#1",
		ContentHash:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotCID:     "bafy-one",
		StoredBy:        "alice",
		StoredAt:        "2026-06-08T12:00:00Z",
		ReceivedAt:      "2026-06-08T12:00:01Z",
		PreviousHash:    nil,
	}
	first.EventHash = hashArchiveNotaryEvent(first)
	prev := first.EventHash
	second := archiveNotaryEvent{
		EventType:       "ARCHIVE_STORED",
		DID:             "did:example:contract:2",
		ContractVersion: 1,
		ArchiveEntryID:  "did:example:contract:2#1",
		ContentHash:     "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		SnapshotCID:     "bafy-two",
		StoredBy:        "bob",
		StoredAt:        "2026-06-08T12:01:00Z",
		ReceivedAt:      "2026-06-08T12:01:01Z",
		PreviousHash:    &prev,
	}
	second.EventHash = hashArchiveNotaryEvent(second)

	events, err := parseArchiveNotaryChain(strings.NewReader(notaryLine(first) + notaryLine(second)))
	if err != nil {
		t.Fatalf("parseArchiveNotaryChain returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if len(events[first.ArchiveEntryID]) != 1 || events[first.ArchiveEntryID][0].EventHash != first.EventHash {
		t.Fatalf("first event hash mismatch")
	}
	if len(events[second.ArchiveEntryID]) != 1 || events[second.ArchiveEntryID][0].PreviousHash == nil || *events[second.ArchiveEntryID][0].PreviousHash != first.EventHash {
		t.Fatalf("second previous hash mismatch")
	}
}

func TestParseArchiveNotaryChainAllowsDuplicateArchiveEntryIDs(t *testing.T) {
	first := archiveNotaryEvent{
		EventType:       "ARCHIVE_STORED",
		DID:             "did:example:contract:1",
		ContractVersion: 1,
		ArchiveEntryID:  "did:example:contract:1#1",
		ContentHash:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotCID:     "bafy-orphaned-retry",
		StoredBy:        "alice",
		StoredAt:        "2026-06-08T12:00:00Z",
		ReceivedAt:      "2026-06-08T12:00:01Z",
	}
	first.EventHash = hashArchiveNotaryEvent(first)
	prev := first.EventHash
	second := archiveNotaryEvent{
		EventType:       "ARCHIVE_STORED",
		DID:             "did:example:contract:1",
		ContractVersion: 1,
		ArchiveEntryID:  "did:example:contract:1#1",
		ContentHash:     "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		SnapshotCID:     "bafy-successful-retry",
		StoredBy:        "alice",
		StoredAt:        "2026-06-08T12:01:00Z",
		ReceivedAt:      "2026-06-08T12:01:01Z",
		PreviousHash:    &prev,
	}
	second.EventHash = hashArchiveNotaryEvent(second)

	events, err := parseArchiveNotaryChain(strings.NewReader(notaryLine(first) + notaryLine(second)))
	if err != nil {
		t.Fatalf("parseArchiveNotaryChain returned error: %v", err)
	}
	candidates := events[first.ArchiveEntryID]
	if len(candidates) != 2 {
		t.Fatalf("expected 2 duplicate candidates, got %d", len(candidates))
	}
	selected, err := selectArchiveNotaryEvent(first.ArchiveEntryID, &archiveNotaryReceiptData{
		ArchiveEntryID: second.ArchiveEntryID,
		EventHash:      second.EventHash,
		PreviousHash:   second.PreviousHash,
		ReceivedAt:     second.ReceivedAt,
	}, candidates)
	if err != nil {
		t.Fatalf("selectArchiveNotaryEvent returned error: %v", err)
	}
	if selected.EventHash != second.EventHash {
		t.Fatalf("selected wrong duplicate event")
	}
}

func TestSelectArchiveNotaryEventComparesEquivalentReceivedAtTimestamps(t *testing.T) {
	event := archiveNotaryEvent{
		EventType:       "ARCHIVE_STORED",
		DID:             "did:example:contract:1",
		ContractVersion: 1,
		ArchiveEntryID:  "did:example:contract:1#1",
		ContentHash:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotCID:     "bafy-one",
		StoredBy:        "alice",
		StoredAt:        "2026-06-30T12:40:20Z",
		ReceivedAt:      "2026-06-30T12:40:21.770Z",
	}
	event.EventHash = hashArchiveNotaryEvent(event)

	selected, err := selectArchiveNotaryEvent(event.ArchiveEntryID, &archiveNotaryReceiptData{
		ArchiveEntryID: event.ArchiveEntryID,
		EventHash:      event.EventHash,
		PreviousHash:   event.PreviousHash,
		ReceivedAt:     "2026-06-30T12:40:21.77Z",
	}, []archiveNotaryEvent{event})
	if err != nil {
		t.Fatalf("selectArchiveNotaryEvent returned error: %v", err)
	}
	if selected.EventHash != event.EventHash {
		t.Fatalf("selected wrong event")
	}
}

func TestParseArchiveNotaryChainRejectsInvalidEventHash(t *testing.T) {
	event := archiveNotaryEvent{
		EventType:       "ARCHIVE_STORED",
		DID:             "did:example:contract:1",
		ContractVersion: 1,
		ArchiveEntryID:  "did:example:contract:1#1",
		ContentHash:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotCID:     "bafy-one",
		StoredBy:        "alice",
		StoredAt:        "2026-06-08T12:00:00Z",
		ReceivedAt:      "2026-06-08T12:00:01Z",
		EventHash:       "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}

	_, err := parseArchiveNotaryChain(strings.NewReader(notaryLine(event)))
	if err == nil {
		t.Fatal("expected invalid event hash error")
	}
}

func TestParseArchiveNotaryChainRejectsInvalidPreviousHash(t *testing.T) {
	invalidPrev := "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	event := archiveNotaryEvent{
		EventType:       "ARCHIVE_STORED",
		DID:             "did:example:contract:1",
		ContractVersion: 1,
		ArchiveEntryID:  "did:example:contract:1#1",
		ContentHash:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotCID:     "bafy-one",
		StoredBy:        "alice",
		StoredAt:        "2026-06-08T12:00:00Z",
		ReceivedAt:      "2026-06-08T12:00:01Z",
		PreviousHash:    &invalidPrev,
	}
	event.EventHash = hashArchiveNotaryEvent(event)

	_, err := parseArchiveNotaryChain(strings.NewReader(notaryLine(event)))
	if err == nil {
		t.Fatal("expected invalid previous hash error")
	}
}

func TestArchiveNotaryChainReaderSendsBearerToken(t *testing.T) {
	event := archiveNotaryEvent{
		EventType:       "ARCHIVE_STORED",
		DID:             "did:example:contract:1",
		ContractVersion: 1,
		ArchiveEntryID:  "did:example:contract:1#1",
		ContentHash:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotCID:     "bafy-one",
		StoredBy:        "alice",
		StoredAt:        "2026-06-08T12:00:00Z",
		ReceivedAt:      "2026-06-08T12:00:01Z",
	}
	event.EventHash = hashArchiveNotaryEvent(event)

	reader := archiveNotaryChainReader{
		url:         "http://orce.example/archive-audit-events.jsonl",
		bearerToken: "test-token",
		httpClient: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("unexpected Authorization header %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(notaryLine(event))),
				Header:     make(http.Header),
			}, nil
		})},
	}
	events, err := reader.Read(context.Background())
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestArchiveSnapshotHashDetectsChangedSnapshot(t *testing.T) {
	snapshot, err := datatype.NewJSON(map[string]any{"did": "did:example:1", "state": "APPROVED"})
	if err != nil {
		t.Fatalf("NewJSON returned error: %v", err)
	}
	changed, err := datatype.NewJSON(map[string]any{"did": "did:example:1", "state": "DRAFT"})
	if err != nil {
		t.Fatalf("NewJSON returned error: %v", err)
	}
	first, err := cwecommand.HashArchiveSnapshot(snapshot)
	if err != nil {
		t.Fatalf("HashArchiveSnapshot returned error: %v", err)
	}
	second, err := cwecommand.HashArchiveSnapshot(snapshot)
	if err != nil {
		t.Fatalf("HashArchiveSnapshot returned error: %v", err)
	}
	third, err := cwecommand.HashArchiveSnapshot(changed)
	if err != nil {
		t.Fatalf("HashArchiveSnapshot returned error: %v", err)
	}
	if first != second {
		t.Fatal("expected deterministic hash")
	}
	if first == third {
		t.Fatal("expected changed snapshot to produce a different hash")
	}
}

func TestArchiveSnapshotHashCanonicalizesJSON(t *testing.T) {
	first := datatype.JSON(`{"did":"did:example:1","state":"APPROVED","contract_data":{"b":2,"a":1}}`)
	second := datatype.JSON(`{
		"contract_data": {
			"a": 1,
			"b": 2
		},
		"state": "APPROVED",
		"did": "did:example:1"
	}`)

	firstHash, err := cwecommand.HashArchiveSnapshot(first)
	if err != nil {
		t.Fatalf("HashArchiveSnapshot returned error: %v", err)
	}
	secondHash, err := cwecommand.HashArchiveSnapshot(second)
	if err != nil {
		t.Fatalf("HashArchiveSnapshot returned error: %v", err)
	}
	if firstHash != secondHash {
		t.Fatalf("expected canonical JSON hashes to match: %s != %s", firstHash, secondHash)
	}
}

func TestFindArchiveStoreEventRequiresNotaryReceipt(t *testing.T) {
	entry := db.ContractArchiveEntry{
		DID:             "did:example:contract:1",
		ContractVersion: 1,
		ContentHash:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotCID:     "bafy-one",
	}
	eventData, err := json.Marshal(archiveStoreEventData{
		DID:             entry.DID,
		ContractVersion: entry.ContractVersion,
		ContentHash:     entry.ContentHash,
		SnapshotCID:     entry.SnapshotCID,
	})
	if err != nil {
		t.Fatalf("marshal event data: %v", err)
	}

	_, _, _, err = findArchiveStoreEvent(entry, "did:example:contract:1#1", []datatype.AuditLogEntry{{
		ID:        1,
		EventType: "STORE_ARCHIVED_CONTRACT",
		EventData: eventData,
	}})
	if err == nil {
		t.Fatal("expected missing notary receipt error")
	}
}

func TestFindArchiveStoreEventReturnsNotaryReceipt(t *testing.T) {
	entry := db.ContractArchiveEntry{
		DID:             "did:example:contract:1",
		ContractVersion: 1,
		ContentHash:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SnapshotCID:     "bafy-one",
	}
	previousHash := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	eventData, err := json.Marshal(archiveStoreEventData{
		DID:             entry.DID,
		ContractVersion: entry.ContractVersion,
		ContentHash:     entry.ContentHash,
		SnapshotCID:     entry.SnapshotCID,
		NotaryReceipt: &archiveNotaryReceiptData{
			ReceiptType:    "ARCHIVE_NOTARY_RECEIPT",
			ArchiveEntryID: "did:example:contract:1#1",
			EventHash:      "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			PreviousHash:   &previousHash,
			ReceivedAt:     "2026-06-08T12:00:01Z",
		},
	})
	if err != nil {
		t.Fatalf("marshal event data: %v", err)
	}

	_, receipt, _, err := findArchiveStoreEvent(entry, "did:example:contract:1#1", []datatype.AuditLogEntry{{
		ID:        1,
		EventType: "STORE_ARCHIVED_CONTRACT",
		EventData: eventData,
	}})
	if err != nil {
		t.Fatalf("findArchiveStoreEvent returned error: %v", err)
	}
	if receipt.EventHash != "sha256:1111111111111111111111111111111111111111111111111111111111111111" {
		t.Fatalf("unexpected receipt event hash %q", receipt.EventHash)
	}
}

func TestReadArchiveTSAReceiptRequiresDBReceipt(t *testing.T) {
	entry := db.ContractArchiveEntry{
		DID:             "did:example:contract:1",
		ContractVersion: 1,
	}

	_, err := readArchiveTSAReceipt(entry, &archiveTSAReceiptData{
		Token:          "token",
		TokenEncoding:  "base64",
		MessageImprint: "abc",
	})
	if err == nil {
		t.Fatal("expected missing DB TSA receipt error")
	}
}

func notaryLine(event archiveNotaryEvent) string {
	previousHash := "null"
	if event.PreviousHash != nil {
		previousHash = fmt.Sprintf("%q", *event.PreviousHash)
	}
	return fmt.Sprintf(
		`{"eventType":%q,"did":%q,"contractVersion":%d,"archiveEntryId":%q,"contentHash":%q,"snapshotCid":%q,"storedBy":%q,"storedAt":%q,"receivedAt":%q,"previousHash":%s,"eventHash":%q}`+"\n",
		event.EventType,
		event.DID,
		event.ContractVersion,
		event.ArchiveEntryID,
		event.ContentHash,
		event.SnapshotCID,
		event.StoredBy,
		event.StoredAt,
		event.ReceivedAt,
		previousHash,
		event.EventHash,
	)
}
