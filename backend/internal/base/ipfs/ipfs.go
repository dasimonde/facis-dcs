// Package ipfs is the client for the IPFS anchor store used by the
// tamper-evident audit trail (base/event.OutboxProcessor writes each signed,
// hash-chained audit entry here) and by C2PA/provenance artifacts
// (pdfgeneration, signingmanagement).
package ipfs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"
)

type APIClient struct {
	// mfsBaseURL is the Kubo RPC API. Artifacts are stored and read through it
	// directly: the XFSC ipfs-document-manager that used to sit in front of it
	// answered every read by listing the whole pinset under Kubo's pinner lock,
	// and gave an add and its pin one shared 5s deadline (ADR-36).
	mfsBaseURL string
	client     *http.Client
}

func NewClient(mfsBaseURL string) *APIClient {
	return &APIClient{
		mfsBaseURL: mfsBaseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type IPFSResult struct {
	Identifier struct {
		Format string `json:"Format"`
		Value  string `json:"Value"`
	} `json:"identifier"`
	Data json.RawMessage `json:"data"`
}

func (c *APIClient) CreateFile(ctx context.Context, data any) (*IPFSResult, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}
	// Raw bytes are stored verbatim: an artifact handed over as []byte is
	// already its final on-disk form (a PDF, a ciphertext blob), and marshalling
	// it would store a base64 JSON string a third larger than the bytes it
	// wraps. Everything else is a value that has no form until it is encoded.
	if raw, ok := data.([]byte); ok {
		body = raw
	}
	return c.createKuboFile(ctx, body)
}

func (c *APIClient) FetchFile(cid string) (*IPFSResult, error) {
	return c.fetchKuboFile(cid)
}

func (c *APIClient) DeleteFile(cid string) error {
	return c.deleteKuboFile(cid)
}

func (c *APIClient) createKuboFile(ctx context.Context, data []byte) (*IPFSResult, error) {
	if c.mfsBaseURL == "" {
		return nil, fmt.Errorf("IPFS_MFS_BASE_URL is required")
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
		return nil, fmt.Errorf("IPFS_MFS_BASE_URL is required")
	}

	// offline=true because every CID this service reads is one it stored
	// itself. Without it a block the node does not hold sends kubo to the
	// network to look for it, and a node configured with no routing and no
	// peers has nowhere to look — so instead of answering "missing" it waits
	// until the caller's own timeout expires. An audit chain walk that meets
	// one dangling CID then spends the whole request budget on it. Offline,
	// a present block reads exactly as before and a missing one fails at once.
	url := fmt.Sprintf("%s/api/v0/cat?arg=%s&offline=true", c.mfsBaseURL, cid)
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

	// The block is returned as stored. Nothing unwraps it: CreateFile writes a
	// []byte artifact verbatim and everything else as the JSON it marshals to,
	// so there is no envelope here to strip. The base64 decode that used to sit
	// in this spot belonged to the tenant manager's response format, and once
	// that reader was gone it could only misfire -- a payload that happened to
	// marshal to a JSON string would have been base64-decoded into garbage.
	result := &IPFSResult{
		Data: body,
	}
	result.Identifier.Format = "CID"
	result.Identifier.Value = cid

	return result, nil
}

func (c *APIClient) deleteKuboFile(cid string) error {
	if c.mfsBaseURL == "" {
		return fmt.Errorf("IPFS_MFS_BASE_URL is required")
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
