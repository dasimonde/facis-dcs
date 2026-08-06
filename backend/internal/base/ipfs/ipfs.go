// Package ipfs is the client for the IPFS anchor store used by the
// tamper-evident audit trail (base/event.OutboxProcessor writes each signed,
// hash-chained audit entry here) and by C2PA/provenance artifacts
// (pdfgeneration, signingmanagement).
package ipfs

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

// maxConcurrentWrites bounds how many stores may be in flight at once. The
// node behind the document manager degrades under concurrent pinning: once one
// pin outlives the manager's own client timeout, every further caller that
// piles on keeps the node saturated and the failure feeds itself — the cap
// turns that stampede into a queue the node can drain.
const maxConcurrentWrites = 4

type APIClient struct {
	baseURL    string
	mfsBaseURL string
	client     *http.Client
	// writeSlots implements the maxConcurrentWrites bound; bulkSlots the
	// tighter maxConcurrentBulkWrites share of it.
	writeSlots chan struct{}
	bulkSlots  chan struct{}
	// fetchAttempts and fetchBackoff bound the read-after-write retry against
	// the tenant store, which is eventually consistent: a CID returned by
	// CreateFile is not always immediately resolvable through the tenant
	// gateway (a subsequent GET can transiently return 404/5xx until the
	// DataIdentifier record and its blocks propagate).
	fetchAttempts int
	fetchBackoff  time.Duration
}

func NewClient(baseURL string, mfsBaseURL string) *APIClient {
	return &APIClient{
		baseURL:    baseURL,
		mfsBaseURL: mfsBaseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		fetchAttempts: 8,
		fetchBackoff:  500 * time.Millisecond,
		writeSlots:    make(chan struct{}, maxConcurrentWrites),
		bulkSlots:     make(chan struct{}, maxConcurrentBulkWrites),
	}
}

// acquireWriteSlot blocks until a write slot frees or the context ends.
func (c *APIClient) acquireWriteSlot(ctx context.Context) error {
	select {
	case c.writeSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("waiting for an ipfs write slot: %w", ctx.Err())
	}
}

func (c *APIClient) releaseWriteSlot() {
	<-c.writeSlots
}

// maxConcurrentBulkWrites is the share of the write pool a background batch may
// hold. Anchoring drains hundreds of queued events concurrently, and with the
// whole pool in its hands the store a signing ceremony is waiting on queues
// behind bulk work nobody is watching — the smaller bound keeps slots free for
// the callers with a user on the other end.
const maxConcurrentBulkWrites = 2

// CreateFileBulk is CreateFile for background batch work: it takes a bulk slot
// before competing for the shared pool, so at most maxConcurrentBulkWrites of
// the batch are ever in flight and interactive writes always find room.
func (c *APIClient) CreateFileBulk(ctx context.Context, data any) (*IPFSResult, error) {
	select {
	case c.bulkSlots <- struct{}{}:
		defer func() { <-c.bulkSlots }()
	case <-ctx.Done():
		return nil, fmt.Errorf("waiting for a bulk ipfs write slot: %w", ctx.Err())
	}
	return c.CreateFile(ctx, data)
}

type IPFSResult struct {
	Identifier struct {
		Format string `json:"Format"`
		Value  string `json:"Value"`
	} `json:"identifier"`
	Data json.RawMessage `json:"data"`
}

func (c *APIClient) CreateFile(ctx context.Context, data any) (*IPFSResult, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}

	if err := c.acquireWriteSlot(ctx); err != nil {
		return nil, err
	}
	defer c.releaseWriteSlot()

	if c.baseURL == "" {
		return c.createKuboFile(ctx, jsonData)
	}

	body := jsonData
	if raw, ok := data.([]byte); ok {
		body = raw
	}

	result, err := c.createTenantFileWithRetry(ctx, body)
	if err != nil {
		return nil, err
	}

	if c.mfsBaseURL != "" {
		err := c.copyToMFS(ctx, c.mfsBaseURL, result.Identifier.Value, result.Identifier.Value)
		if err != nil {
			return result, err
		}
	}

	return result, nil
}

// createTenantFileWithRetry stores bytes through the tenant document manager,
// retrying transport failures and 5xx the same way reads already retry.
//
// The document manager pins to its IPFS node as part of the call, and a pin is
// a network hop that can fail transiently under load — a single blip otherwise
// fails the whole signing. Retrying is safe because the store is content
// addressed: the same bytes always yield the same CID, so a retried write
// converges on the object the first attempt was creating.
func (c *APIClient) createTenantFileWithRetry(ctx context.Context, body []byte) (*IPFSResult, error) {
	url := c.baseURL + "/api/ipfs/create"

	attempts := c.fetchAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && c.fetchBackoff > 0 {
			// Exponential, capped: a flat cadence keeps a struggling node at
			// constant pressure, and a stampede of callers retrying in lockstep
			// is what turned one slow pin into a stall nothing recovered from.
			delay := c.fetchBackoff << (attempt - 1)
			if max := 8 * time.Second; delay > max {
				delay = max
			}
			time.Sleep(delay)
		}

		result, status, err := c.createTenantOnce(ctx, url, body)
		if err != nil {
			lastErr = err
			continue
		}
		if status == http.StatusOK {
			return result, nil
		}
		lastErr = fmt.Errorf("unexpected status %d", status)
		// A 4xx is a definitive answer about these bytes; only the server-side
		// transients are worth another attempt.
		if status < 500 {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

func (c *APIClient) createTenantOnce(ctx context.Context, url string, body []byte) (*IPFSResult, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, bodyBytes)
	}

	var result IPFSResult
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode response: %w", err)
	}
	return &result, resp.StatusCode, nil
}

func (c *APIClient) FetchFile(cid string) (*IPFSResult, error) {
	if c.baseURL == "" {
		return c.fetchKuboFile(cid)
	}

	// Fast path: one tenant attempt, then the durable pinned Kubo copy. Both are
	// written synchronously by CreateFile (copyToMFS), so the common case — the
	// tenant document-manager dropped its DataIdentifier mapping under load —
	// resolves in two quick calls. Doing the multi-second tenant retry first
	// would compound over a long audit-chain walk into a request-deadline
	// timeout (DCS-FR-CSA: the tamper-proof trail read must not 404 or hang on a
	// link the tenant index transiently forgot; the Kubo copy is identical and
	// the hash chain still verifies).
	if body, status, err := c.getOnce(fmt.Sprintf("%s/api/ipfs/%s", c.baseURL, cid)); err == nil && status == http.StatusOK {
		return decodeTenantBody(body)
	}
	if c.mfsBaseURL != "" {
		if kubo, kerr := c.fetchKuboFile(cid); kerr == nil {
			return kubo, nil
		}
	}

	// Neither resolved on the first try — treat as genuine read-after-write lag
	// and retry the tenant path with backoff, falling back to Kubo once more.
	body, err := c.fetchTenantFileWithRetry(cid)
	if err != nil {
		if c.mfsBaseURL != "" {
			if kubo, kerr := c.fetchKuboFile(cid); kerr == nil {
				return kubo, nil
			}
		}
		return nil, err
	}
	return decodeTenantBody(body)
}

// decodeTenantBody unwraps a tenant-gateway response into an IPFSResult,
// decoding the base64-in-JSON-string data payload the tenant store wraps.
func decodeTenantBody(body []byte) (*IPFSResult, error) {
	var result IPFSResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(result.Data) > 0 {
		var dataStr string
		if err := json.Unmarshal(result.Data, &dataStr); err != nil {
			return nil, fmt.Errorf("decode ipfs data json string: %w", err)
		}
		if dataStr == "" {
			return nil, fmt.Errorf("decode ipfs data: empty payload")
		}
		decoded, err := base64.StdEncoding.DecodeString(dataStr)
		if err != nil {
			return nil, fmt.Errorf("decode ipfs data base64: %w", err)
		}
		result.Data = json.RawMessage(decoded)
	}

	return &result, nil
}

// fetchTenantFileWithRetry GETs a CID from the tenant gateway, retrying on
// transient not-yet-resolvable responses (404/5xx) with a bounded backoff.
// This absorbs the tenant store's read-after-write lag so a CID that CreateFile
// has just returned is reliably retrievable by a subsequent request.
func (c *APIClient) fetchTenantFileWithRetry(cid string) ([]byte, error) {
	url := fmt.Sprintf("%s/api/ipfs/%s", c.baseURL, cid)

	attempts := c.fetchAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 && c.fetchBackoff > 0 {
			time.Sleep(c.fetchBackoff)
		}

		body, status, err := c.getOnce(url)
		if err != nil {
			lastErr = err
			continue
		}
		if status == http.StatusOK {
			return body, nil
		}
		lastErr = fmt.Errorf("unexpected status %d: %s", status, body)
		// Only the transient not-yet-resolvable statuses are worth retrying;
		// any other 4xx is a definitive answer.
		if status != http.StatusNotFound && status < 500 {
			return nil, lastErr
		}
	}
	return nil, lastErr
}

func (c *APIClient) getOnce(url string) ([]byte, int, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, 0, err
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (c *APIClient) DeleteFile(cid string) error {
	if c.baseURL == "" {
		return c.deleteKuboFile(cid)
	}

	url := fmt.Sprintf("%s/api/ipfs/%s", c.baseURL, cid)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, url, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)

	return nil

}

func (c *APIClient) createKuboFile(ctx context.Context, data []byte) (*IPFSResult, error) {
	if c.mfsBaseURL == "" {
		return nil, fmt.Errorf("IPFS_MFS_BASE_URL is required when IPFS_TENANT_BASE_URL is not configured")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="file"; filename="audit-log.json"`)
	header.Set("Content-Type", "application/json")

	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, fmt.Errorf("create multipart part: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("write multipart data: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	url := c.mfsBaseURL + "/api/v0/add?pin=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &body)
	if err != nil {
		return nil, fmt.Errorf("create Kubo add request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do Kubo add request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected Kubo add status %d: %s", resp.StatusCode, body)
	}

	var addResult struct {
		Hash string `json:"Hash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&addResult); err != nil {
		return nil, fmt.Errorf("decode Kubo add response: %w", err)
	}
	if addResult.Hash == "" {
		return nil, fmt.Errorf("the Kubo add response does not include a CID")
	}

	result := &IPFSResult{
		Data: data,
	}
	result.Identifier.Format = "CID"
	result.Identifier.Value = addResult.Hash

	if err := c.copyToMFS(ctx, c.mfsBaseURL, addResult.Hash, addResult.Hash); err != nil {
		return result, err
	}

	return result, nil
}

func (c *APIClient) fetchKuboFile(cid string) (*IPFSResult, error) {
	if c.mfsBaseURL == "" {
		return nil, fmt.Errorf("IPFS_MFS_BASE_URL is required when IPFS_TENANT_BASE_URL is not configured")
	}

	url := fmt.Sprintf("%s/api/v0/cat?arg=%s", c.mfsBaseURL, cid)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create Kubo cat request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do Kubo cat request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected Kubo cat status %d: %s", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Kubo cat response: %w", err)
	}

	var dataStr string
	var resultData []byte
	if json.Unmarshal(body, &dataStr) == nil {
		decoded, err := base64.StdEncoding.DecodeString(dataStr)
		if err != nil {
			return nil, fmt.Errorf("base64 decode Kubo file data: %w", err)
		}
		resultData = decoded
	} else {
		resultData = body
	}

	result := &IPFSResult{
		Data: resultData,
	}
	result.Identifier.Format = "CID"
	result.Identifier.Value = cid

	return result, nil
}

func (c *APIClient) deleteKuboFile(cid string) error {
	if c.mfsBaseURL == "" {
		return fmt.Errorf("IPFS_MFS_BASE_URL is required when IPFS_TENANT_BASE_URL is not configured")
	}

	url := fmt.Sprintf("%s/api/v0/pin/rm?arg=%s", c.mfsBaseURL, cid)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create Kubo unpin request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("do Kubo unpin request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected Kubo unpin status %d: %s", resp.StatusCode, body)
	}

	return nil
}

func (c *APIClient) copyToMFS(ctx context.Context, baseURL string, cid string, filename string) error {

	url := fmt.Sprintf("%s/api/v0/files/cp?arg=/ipfs/%s&arg=/%s", baseURL, cid, filename)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	// files/cp fails when /<filename> already exists in MFS. In the shared-IPFS
	// federation a peer instance may have already copied this exact CID: the
	// store is content-addressed, so an existing entry at the same path holds
	// identical bytes and the desired postcondition already holds. Confirm the
	// entry resolves to the same CID and treat that as success rather than
	// rolling back the caller's work over a benign collision.
	if c.mfsEntryHasCID(ctx, baseURL, filename, cid) {
		return nil
	}
	return fmt.Errorf("unexpected Kubo files/cp status %d: %s", resp.StatusCode, body)
}

// mfsEntryHasCID reports whether the MFS path /<filename> already resolves to
// the given CID (via files/stat). Used to make copyToMFS idempotent: a
// content-addressed entry that is already present holds the same bytes.
func (c *APIClient) mfsEntryHasCID(ctx context.Context, baseURL string, filename string, cid string) bool {
	url := fmt.Sprintf("%s/api/v0/files/stat?arg=/%s", baseURL, filename)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer func(Body io.ReadCloser) {
		if err := Body.Close(); err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return false
	}
	var stat struct {
		Hash string `json:"Hash"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&stat); err != nil {
		return false
	}
	return stat.Hash == cid
}
