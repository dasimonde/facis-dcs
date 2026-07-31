package auditexecutor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ContractVersion  = "facis-pac-audit-executor/v1"
	maxResponseBytes = 1 << 20
)

type Resource struct {
	DID string `json:"did"`
}

type Requester struct {
	Subject string   `json:"subject"`
	Roles   []string `json:"roles"`
}

type Request struct {
	ContractVersion string         `json:"contract_version"`
	AuditID         string         `json:"audit_id"`
	CorrelationID   string         `json:"correlation_id"`
	Scope           string         `json:"scope"`
	Resource        *Resource      `json:"resource,omitempty"`
	Requester       Requester      `json:"requester"`
	Justification   string         `json:"justification"`
	Evidence        map[string]any `json:"evidence"`
}

type Executor struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type Finding struct {
	RuleID string `json:"rule_id"`
	// Result is the executor's verdict. NOT_EVALUATED is the fourth value
	// ADR-33 requires: the rule was carried into the audit but nobody reached
	// a verdict on it, which PASSED / FAILED / REVIEW cannot express without
	// claiming a check that never ran.
	Result       string   `json:"result"`
	Reason       string   `json:"reason"`
	Severity     string   `json:"severity,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

type Response struct {
	ContractVersion string          `json:"contract_version"`
	AuditID         string          `json:"audit_id"`
	CorrelationID   string          `json:"correlation_id"`
	Scope           string          `json:"scope"`
	Resource        *Resource       `json:"resource,omitempty"`
	Executor        Executor        `json:"executor"`
	ExecutedAt      string          `json:"executed_at"`
	Findings        []Finding       `json:"findings"`
	Receipt         json.RawMessage `json:"receipt,omitempty"`
}

type Client interface {
	Run(context.Context, Request) (Response, []byte, error)
}

type HTTPClient struct {
	url         string
	bearerToken string
	httpClient  *http.Client
}

func NewHTTPClient(url, bearerToken string, timeout time.Duration) (*HTTPClient, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, errors.New("audit executor URL is required")
	}
	if timeout <= 0 {
		return nil, errors.New("audit executor timeout must be positive")
	}
	return &HTTPClient{
		url:         url,
		bearerToken: strings.TrimSpace(bearerToken),
		httpClient:  &http.Client{Timeout: timeout},
	}, nil
}

func (c *HTTPClient) Run(ctx context.Context, request Request) (Response, []byte, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return Response{}, nil, fmt.Errorf("encode audit executor request: %w", err)
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return Response{}, nil, fmt.Errorf("create audit executor request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	if c.bearerToken != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}

	// Deliberately exactly one outbound attempt. Audit execution is not retried:
	// the caller owns the audit UUID and can explicitly submit another run.
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return Response{}, nil, fmt.Errorf("call audit executor: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return Response{}, nil, fmt.Errorf("read audit executor response: %w", err)
	}
	if len(raw) > maxResponseBytes {
		return Response{}, nil, errors.New("audit executor response exceeds 1 MiB")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Response{}, nil, fmt.Errorf("audit executor returned HTTP %d", response.StatusCode)
	}

	var result Response
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return Response{}, nil, fmt.Errorf("decode audit executor response: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Response{}, nil, err
	}
	if err := ValidateResponse(request, result); err != nil {
		return Response{}, nil, err
	}
	return result, raw, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("audit executor response contains multiple JSON values")
		}
		return fmt.Errorf("decode trailing audit executor response: %w", err)
	}
	return nil
}

func ValidateResponse(request Request, response Response) error {
	if response.ContractVersion != ContractVersion || response.ContractVersion != request.ContractVersion {
		return errors.New("audit executor contract version mismatch")
	}
	if _, err := uuid.Parse(response.AuditID); err != nil || response.AuditID != request.AuditID {
		return errors.New("audit executor audit ID mismatch")
	}
	if _, err := uuid.Parse(response.CorrelationID); err != nil || response.CorrelationID != request.CorrelationID {
		return errors.New("audit executor correlation ID mismatch")
	}
	if response.Scope != request.Scope {
		return errors.New("audit executor scope mismatch")
	}
	if !sameResource(request.Resource, response.Resource) {
		return errors.New("audit executor resource mismatch")
	}
	if strings.TrimSpace(response.Executor.ID) == "" || strings.TrimSpace(response.Executor.Version) == "" {
		return errors.New("audit executor identity is incomplete")
	}
	if _, err := time.Parse(time.RFC3339, response.ExecutedAt); err != nil {
		return errors.New("audit executor execution time is invalid")
	}
	if response.Findings == nil {
		return errors.New("audit executor findings must be an array")
	}
	for index, finding := range response.Findings {
		if strings.TrimSpace(finding.RuleID) == "" || strings.TrimSpace(finding.Reason) == "" {
			return fmt.Errorf("audit executor finding %d is incomplete", index)
		}
		switch finding.Result {
		case "PASSED", "FAILED", "REVIEW", "NOT_EVALUATED":
		default:
			return fmt.Errorf("audit executor finding %d has invalid result", index)
		}
	}
	return nil
}

func sameResource(left, right *Resource) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.DID == right.DID
}
