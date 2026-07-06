package command

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

type ArchiveNotaryPayload struct {
	EventType       string    `json:"eventType"`
	ArchiveEntryID  string    `json:"archiveEntryId"`
	DID             string    `json:"did"`
	ContractVersion int       `json:"contractVersion"`
	ContentHash     string    `json:"contentHash"`
	SnapshotCID     string    `json:"snapshotCid"`
	StoredBy        string    `json:"storedBy"`
	StoredAt        time.Time `json:"storedAt"`
}

type ArchiveNotaryReceipt struct {
	ReceiptType    string    `json:"receiptType"`
	ArchiveEntryID string    `json:"archiveEntryId"`
	EventHash      string    `json:"eventHash"`
	PreviousHash   *string   `json:"previousHash"`
	ReceivedAt     time.Time `json:"receivedAt"`
}

type HTTPArchiveNotaryClient struct {
	url        string
	httpClient *http.Client
}

func NewHTTPArchiveNotaryClient(url string) *HTTPArchiveNotaryClient {
	return &HTTPArchiveNotaryClient{
		url: strings.TrimSpace(url),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *HTTPArchiveNotaryClient) NotarizeArchiveEntry(ctx context.Context, payload ArchiveNotaryPayload) (*ArchiveNotaryReceipt, error) {
	if c == nil || c.url == "" {
		return nil, fmt.Errorf("archive notary URL is empty")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal archive notary request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create archive notary request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post archive notary request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read archive notary response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("archive notary returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var receipt ArchiveNotaryReceipt
	if err := json.Unmarshal(respBody, &receipt); err != nil {
		return nil, fmt.Errorf("unmarshal archive notary response: %w", err)
	}
	if receipt.EventHash == "" {
		return nil, fmt.Errorf("archive notary response has empty event hash")
	}
	if receipt.ArchiveEntryID == "" {
		receipt.ArchiveEntryID = payload.ArchiveEntryID
	}
	if receipt.ReceiptType == "" {
		receipt.ReceiptType = "ARCHIVE_NOTARY_RECEIPT"
	}

	return &receipt, nil
}

func archiveNotaryEntryID(did string, contractVersion int) string {
	return fmt.Sprintf("%s#%d", did, contractVersion)
}

func archiveNotaryEventReceipt(receipt *ArchiveNotaryReceipt) *contractevents.ArchiveNotaryReceipt {
	if receipt == nil {
		return nil
	}
	return &contractevents.ArchiveNotaryReceipt{
		ReceiptType:    receipt.ReceiptType,
		ArchiveEntryID: receipt.ArchiveEntryID,
		EventHash:      receipt.EventHash,
		PreviousHash:   receipt.PreviousHash,
		ReceivedAt:     receipt.ReceivedAt,
	}
}
