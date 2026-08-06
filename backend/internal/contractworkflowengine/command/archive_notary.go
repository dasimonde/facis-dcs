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
	url         string
	bearerToken string
	httpClient  *http.Client
}

func NewHTTPArchiveNotaryClient(url, bearerToken string) *HTTPArchiveNotaryClient {
	return &HTTPArchiveNotaryClient{
		url:         strings.TrimSpace(url),
		bearerToken: strings.TrimSpace(bearerToken),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// ValidateArchiveNotaryConfig reports a notary that is addressed but cannot
// authenticate. The two settings come from different places — the URL is
// derived from the bundled ORCE release whenever it is enabled, the token is an
// operator secret that defaults to empty — so the combination "URL set, token
// missing" is what a deployment reaches by configuring nothing at all.
//
// It is checked at startup because of where it would otherwise surface: the
// notary receipt is taken while the applied signature is being archived, which
// is the last step of the whole contract lifecycle. The deployment comes up
// healthy, every screen works, a signer completes the wallet ceremony, and only
// the upload of the signed document is refused — after the signature exists.
// The endpoint answering 401 to an empty bearer is the same verdict either way
// (charts/orce archive-flow.json refuses an empty configured token), so nothing
// is lost by saying it while the pod is starting.
func ValidateArchiveNotaryConfig(url, bearerToken string) error {
	if strings.TrimSpace(url) == "" {
		return nil
	}
	if strings.TrimSpace(bearerToken) == "" {
		return fmt.Errorf(
			"archive notary is configured at %q but its bearer token is empty: set global.archiveAuditToken "+
				"(or global.archiveAuditTokenSecretRef) — the same value authorizes the DCS and the ORCE "+
				"archive flow, and without it every signature is refused when its archive entry is notarized",
			strings.TrimSpace(url),
		)
	}
	return nil
}

func (c *HTTPArchiveNotaryClient) NotarizeArchiveEntry(ctx context.Context, payload ArchiveNotaryPayload) (*ArchiveNotaryReceipt, error) {
	if c == nil || c.url == "" {
		return nil, fmt.Errorf("archive notary URL is empty")
	}
	if c.bearerToken == "" {
		return nil, fmt.Errorf("archive notary bearer token is empty")
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
	req.Header.Set("Authorization", "Bearer "+c.bearerToken)

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
