package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	goahttp "goa.design/goa/v3/http"

	archivesrv "digital-contracting-service/gen/http/contract_storage_archive/server"
	workflowsrv "digital-contracting-service/gen/http/contract_workflow_engine/server"
	pacsrv "digital-contracting-service/gen/http/process_audit_and_compliance/server"
	signaturesrv "digital-contracting-service/gen/http/signature_management/server"
	templatesrv "digital-contracting-service/gen/http/template_repository/server"
)

// Every endpoint that reads an audit log refuses a request without a
// justification, so the access-attempt log always records why the audit log
// was read (DCS-FR-PACM-04, DCS-FR-UC-07-2).
func TestEveryAuditEndpointRequiresAJustification(t *testing.T) {
	mux := goahttp.NewMuxer()
	dec := goahttp.RequestDecoder
	asErr := func(_ any, err error) error { return err }
	endpoints := []struct {
		name, method, path, body string
		decode                   func(*http.Request) error
	}{
		{"signature", http.MethodGet, "/signature/audit?did=did:x", "",
			func(r *http.Request) error { return asErr(signaturesrv.DecodeAuditRequest(mux, dec)(r)) }},
		{"template", http.MethodGet, "/template/audit?did=did:x", "",
			func(r *http.Request) error { return asErr(templatesrv.DecodeAuditRequest(mux, dec)(r)) }},
		{"contract", http.MethodPost, "/contract/audit", `{"did":"did:x"}`,
			func(r *http.Request) error { return asErr(workflowsrv.DecodeAuditRequest(mux, dec)(r)) }},
		{"archive", http.MethodGet, "/archive/audit?did=did:x", "",
			func(r *http.Request) error { return asErr(archivesrv.DecodeAuditRequest(mux, dec)(r)) }},
		{"pac", http.MethodPost, "/pac/audit", `{"scope":"contract"}`,
			func(r *http.Request) error { return asErr(pacsrv.DecodeAuditRequest(mux, dec)(r)) }},
		{"pac report", http.MethodGet, "/pac/report?scope=contract", "",
			func(r *http.Request) error { return asErr(pacsrv.DecodeAuditReportRequest(mux, dec)(r)) }},
		{"access attempts", http.MethodGet, "/pac/access-attempts?limit=5", "",
			func(r *http.Request) error { return asErr(pacsrv.DecodeAccessAttemptsRequest(mux, dec)(r)) }},
		{"case file", http.MethodGet, "/pac/case/c1?x=1", "",
			func(r *http.Request) error { return asErr(pacsrv.DecodeCaseFileRequest(mux, dec)(r)) }},
		{"archive delete", http.MethodDelete, "/archive/delete?did=did:x", "",
			func(r *http.Request) error { return asErr(archivesrv.DecodeDeleteRequest(mux, dec)(r)) }},
		{"workflow gate review", http.MethodPost, "/pac/workflow-gates/review", `{"run_id":"r","decision":"approve"}`,
			func(r *http.Request) error { return asErr(pacsrv.DecodeWorkflowGateReviewRequest(mux, dec)(r)) }},
	}
	request := func(method, path, body string) *http.Request {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer token")
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		return r
	}
	for _, e := range endpoints {
		if err := e.decode(request(e.method, e.path, e.body)); err == nil || !strings.Contains(err.Error(), "justification") {
			t.Errorf("%s audit without justification: got %v, want a missing-justification error", e.name, err)
		}
		with := func(query, json string) *http.Request {
			path, body := e.path, e.body
			if body == "" {
				sep := "?"
				if strings.Contains(path, "?") {
					sep = "&"
				}
				path += sep + "justification=" + query
			} else {
				body = strings.TrimSuffix(body, "}") + `,"justification":"` + json + `"}`
			}
			return request(e.method, path, body)
		}
		if err := e.decode(with("%20%09", ` \t`)); err == nil || !strings.Contains(err.Error(), "justification") {
			t.Errorf("%s audit with a blank justification: got %v, want a justification error", e.name, err)
		}
		if err := e.decode(with("investigation", "investigation")); err != nil {
			t.Errorf("%s audit with justification: %v", e.name, err)
		}
	}
}
