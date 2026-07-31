package auditexecutor

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func validRequest() Request {
	id := uuid.NewString()
	return Request{
		ContractVersion: ContractVersion,
		AuditID:         id, CorrelationID: id, Scope: "contracts",
		Resource:      &Resource{DID: "did:web:example.test:contract"},
		Requester:     Requester{Subject: "auditor", Roles: []string{"Auditor"}},
		Justification: "unit test", Evidence: map[string]any{"contracts": []any{}},
	}
}

func validResponse(request Request) Response {
	return Response{
		ContractVersion: ContractVersion, AuditID: request.AuditID,
		CorrelationID: request.CorrelationID, Scope: request.Scope, Resource: request.Resource,
		Executor: Executor{ID: "test", Version: "1"}, ExecutedAt: time.Now().UTC().Format(time.RFC3339),
		Findings: []Finding{},
	}
}

func TestHTTPClientDispatchesExactlyOnceAndValidatesResponse(t *testing.T) {
	request := validRequest()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(Response{
			ContractVersion: ContractVersion, AuditID: request.AuditID,
			CorrelationID: request.CorrelationID, Scope: request.Scope, Resource: request.Resource,
			Executor: Executor{ID: "test", Version: "1"}, ExecutedAt: time.Now().UTC().Format(time.RFC3339),
			Findings: []Finding{},
		})
	}))
	defer server.Close()

	client, err := NewHTTPClient(server.URL, "secret", time.Second)
	require.NoError(t, err)
	result, _, err := client.Run(context.Background(), request)
	require.NoError(t, err)
	require.Empty(t, result.Findings)
	require.EqualValues(t, 1, calls.Load())
}

func TestHTTPClientFailsClosedOnCorrelationMismatch(t *testing.T) {
	request := validRequest()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(Response{
			ContractVersion: ContractVersion, AuditID: request.AuditID,
			CorrelationID: uuid.NewString(), Scope: request.Scope, Resource: request.Resource,
			Executor: Executor{ID: "test", Version: "1"}, ExecutedAt: time.Now().UTC().Format(time.RFC3339),
			Findings: []Finding{},
		})
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, "", time.Second)
	require.NoError(t, err)
	_, _, err = client.Run(context.Background(), request)
	require.ErrorContains(t, err, "correlation ID mismatch")
}

func TestHTTPClientFailsClosedOnInvalidResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       func(Request) []byte
		want       string
	}{
		{name: "non 2xx", statusCode: http.StatusServiceUnavailable, body: func(Request) []byte { return []byte(`{"error":"down"}`) }, want: "HTTP 503"},
		{name: "malformed", statusCode: http.StatusOK, body: func(Request) []byte { return []byte(`{broken`) }, want: "decode audit executor response"},
		{name: "unknown field", statusCode: http.StatusOK, body: func(request Request) []byte {
			raw, _ := json.Marshal(validResponse(request))
			return bytes.Replace(raw, []byte(`"findings":`), []byte(`"unknown":true,"findings":`), 1)
		}, want: "unknown field"},
		{name: "trailing json", statusCode: http.StatusOK, body: func(request Request) []byte {
			raw, _ := json.Marshal(validResponse(request))
			return append(raw, []byte(` {}`)...)
		}, want: "multiple JSON values"},
		{name: "version mismatch", statusCode: http.StatusOK, body: func(request Request) []byte {
			response := validResponse(request)
			response.ContractVersion = "invalid"
			raw, _ := json.Marshal(response)
			return raw
		}, want: "contract version mismatch"},
		{name: "scope mismatch", statusCode: http.StatusOK, body: func(request Request) []byte {
			response := validResponse(request)
			response.Scope = "templates"
			raw, _ := json.Marshal(response)
			return raw
		}, want: "scope mismatch"},
		{name: "resource mismatch", statusCode: http.StatusOK, body: func(request Request) []byte {
			response := validResponse(request)
			response.Resource = &Resource{DID: "did:example:mismatch"}
			raw, _ := json.Marshal(response)
			return raw
		}, want: "resource mismatch"},
		{name: "invalid finding", statusCode: http.StatusOK, body: func(request Request) []byte {
			response := validResponse(request)
			response.Findings = []Finding{{RuleID: "RULE", Result: "INVALID", Reason: "bad"}}
			raw, _ := json.Marshal(response)
			return raw
		}, want: "invalid result"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validRequest()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(test.statusCode)
				_, _ = w.Write(test.body(request))
			}))
			defer server.Close()
			client, err := NewHTTPClient(server.URL, "", time.Second)
			require.NoError(t, err)
			_, _, err = client.Run(context.Background(), request)
			require.ErrorContains(t, err, test.want)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestHTTPClientFailsClosedOnTimeoutWithoutRetry(t *testing.T) {
	request := validRequest()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(validResponse(request))
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, "", 5*time.Millisecond)
	require.NoError(t, err)
	_, _, err = client.Run(context.Background(), request)
	require.ErrorContains(t, err, "call audit executor")
	require.EqualValues(t, 1, calls.Load())
}
