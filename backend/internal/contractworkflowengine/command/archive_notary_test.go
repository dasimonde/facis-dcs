package command

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPArchiveNotaryClientUsesConfiguredBearerToken(t *testing.T) {
	const token = "configured-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("Authorization = %q", got)
		}
		_ = json.NewEncoder(w).Encode(ArchiveNotaryReceipt{ReceiptType: "ARCHIVE_NOTARY_RECEIPT", ArchiveEntryID: "entry", EventHash: "sha256:hash", ReceivedAt: time.Now().UTC()})
	}))
	defer server.Close()

	client := NewHTTPArchiveNotaryClient(server.URL, token)
	if _, err := client.NotarizeArchiveEntry(context.Background(), ArchiveNotaryPayload{ArchiveEntryID: "entry"}); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPArchiveNotaryClientRejectsMissingBearerToken(t *testing.T) {
	client := NewHTTPArchiveNotaryClient("http://example.invalid", "")
	if _, err := client.NotarizeArchiveEntry(context.Background(), ArchiveNotaryPayload{}); err == nil {
		t.Fatal("expected missing token error")
	}
}
