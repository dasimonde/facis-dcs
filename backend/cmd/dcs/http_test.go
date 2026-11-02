package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	goahttp "goa.design/goa/v3/http"

	pdfgeneration "digital-contracting-service/gen/pdf_generation"
)

// DCS-FR-CWE-24: the metrics wrapper lets the server-sent event stream flush
// each event to the client instead of holding it in the response buffer.
func TestMetricsMiddlewareFlushesStreamedResponses(t *testing.T) {
	recorder := httptest.NewRecorder()
	metricsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("data: {}\n\n"))
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush: %v", err)
		}
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/contract/events", nil))
	if !recorder.Flushed {
		t.Fatal("the event stayed in the response buffer")
	}
}

// TestErrorFormatterPreservesBundleExportFindings verifies that a
// *pdfgeneration.BundleExportRefusedError keeps its findings array (and gets a
// 422 status) instead of being collapsed into the generic Goa error hull —
// clients read the findings out of the refusal response body.
func TestErrorFormatterPreservesBundleExportFindings(t *testing.T) {
	findings := []string{
		"contract abc: no exported PDF (export the contract PDF before bundling)",
		"contract abc: exported PDF carries no C2PA manifest store",
	}
	err := &pdfgeneration.BundleExportRefusedError{
		Name:     "refused",
		Message:  "contract bundle export refused by structural-integrity pre-flight",
		Findings: findings,
	}

	statuser := errorFormatter(context.Background(), err)
	if got := statuser.StatusCode(); got != http.StatusUnprocessableEntity {
		t.Fatalf("StatusCode() = %d, want %d", got, http.StatusUnprocessableEntity)
	}

	body, marshalErr := json.Marshal(statuser)
	if marshalErr != nil {
		t.Fatalf("marshal refusal body: %v", marshalErr)
	}

	var decoded struct {
		Name     string   `json:"name"`
		Message  string   `json:"message"`
		Findings []string `json:"findings"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal refusal body: %v", err)
	}

	if decoded.Name != "refused" {
		t.Errorf("name = %q, want %q", decoded.Name, "refused")
	}
	if len(decoded.Findings) != len(findings) {
		t.Fatalf("findings = %v, want %v", decoded.Findings, findings)
	}
	for i, f := range findings {
		if decoded.Findings[i] != f {
			t.Errorf("findings[%d] = %q, want %q", i, decoded.Findings[i], f)
		}
	}
}

// TestErrorFormatterRefusalNilFindings ensures a refusal with no findings still
// serializes a present (empty, non-null) findings array, matching the generated
// response body shape.
func TestErrorFormatterRefusalNilFindings(t *testing.T) {
	err := &pdfgeneration.BundleExportRefusedError{Name: "refused", Message: "refused"}

	body, marshalErr := json.Marshal(errorFormatter(context.Background(), err))
	if marshalErr != nil {
		t.Fatalf("marshal refusal body: %v", marshalErr)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal refusal body: %v", err)
	}
	raw, ok := decoded["findings"]
	if !ok {
		t.Fatalf("findings key missing from refusal body: %s", body)
	}
	if string(raw) != "[]" {
		t.Errorf("findings = %s, want []", raw)
	}
}

// The Webhooks view calls the webhook platform under the API path, which the
// ingress routes to the backend; /orce/ on the root stays for ORCE
// (DCS-NFR-SQ-08).
func TestWebhookPlatformIsUnderTheAPIPath(t *testing.T) {
	t.Setenv("DCS_API_PATH", "/digital-contracting-service/api")
	platform := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("platform " + r.URL.Path)) })
	mux := newOuterMux(platform, http.NotFoundHandler(), http.NotFoundHandler())
	for path, want := range map[string]string{
		"/digital-contracting-service/api/orce/webhooks": "platform /webhooks",
		"/orce/webhooks": "platform /webhooks",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Body.String() != want {
			t.Errorf("GET %s = %d %q, want %q", path, rec.Code, rec.Body.String(), want)
		}
	}
}

// TestMetricsOnlyOnMetricsListener: the public handler chain must not serve
// /metrics; the metrics listener must.
func TestMetricsOnlyOnMetricsListener(t *testing.T) {
	public := newOuterMux(http.NotFoundHandler(), http.NotFoundHandler(), http.NotFoundHandler())
	rec := httptest.NewRecorder()
	public.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("public GET /metrics = %d, want %d", rec.Code, http.StatusNotFound)
	}

	rec = httptest.NewRecorder()
	newMetricsHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "# HELP") {
		t.Errorf("metrics GET /metrics = %d, body %.80q; want 200 with Prometheus text", rec.Code, rec.Body.String())
	}
}

// DCS-NFR-PER-01 (NFR-8): the request metrics are labelled with the route
// pattern and a known method, so requests to unknown paths or with invented
// methods add no new series.
func TestMetricsLabelsStayBounded(t *testing.T) {
	goaMux := goahttp.NewMuxer()
	goaMux.Handle(http.MethodGet, "/contract/{did}", func(w http.ResponseWriter, _ *http.Request) {})
	handler := metricsMiddleware(recordRoute(newOuterMux(http.NotFoundHandler(), http.NotFoundHandler(), goaMux)))
	serve := func(method, path string) {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, path, nil))
	}

	before := seriesCount(httpRequestsTotal)
	for i := range 100 {
		serve(http.MethodPost, fmt.Sprintf("/scan/%d", i))
		serve(http.MethodGet, fmt.Sprintf("/contract/did:example:%d", i))
	}
	serve("FOO", "/contract/did:example:1")
	if added := seriesCount(httpRequestsTotal) - before; added > 3 {
		t.Fatalf("201 requests added %d label sets, want at most 3", added)
	}
	for _, labels := range [][]string{{http.MethodGet, "/contract/{did}", "200"}, {"OTHER", "/", "405"}} {
		if !httpRequestsTotal.DeleteLabelValues(labels...) {
			t.Errorf("no series %v", labels)
		}
	}
}

func seriesCount(c prometheus.Collector) int {
	ch := make(chan prometheus.Metric)
	go func() { c.Collect(ch); close(ch) }()
	n := 0
	for range ch {
		n++
	}
	return n
}
