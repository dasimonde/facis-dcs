package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	goahttp "goa.design/goa/v3/http"

	processauditandcompliancesvr "digital-contracting-service/gen/http/process_audit_and_compliance/server"
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
)

func TestAuditReportHTTPMediaTypesAndErrors(t *testing.T) {
	for _, format := range []string{"json", "csv", "pdf", " CSV "} {
		for _, accept := range []string{"", "application/json", "text/csv", "application/xml"} {
			for _, status := range []int{200, 400, 403, 404, 500} {
				t.Run(format+"/"+accept+"/"+http.StatusText(status), func(t *testing.T) {
					content := []byte("raw report bytes")
					var resultErr error
					switch status {
					case 400:
						resultErr = processauditandcompliance.MakeBadRequest(errors.New("bad format"))
					case 403:
						resultErr = processauditandcompliance.MakeForbidden(errors.New("forbidden"))
					case 404:
						resultErr = processauditandcompliance.MakeNotFound(errors.New("erased"))
					case 500:
						resultErr = processauditandcompliance.MakeInternalError(errors.New("storage failure"))
					}
					handler := processauditandcompliancesvr.NewAuditReportHandler(
						func(context.Context, any) (any, error) { return content, resultErr },
						goahttp.NewMuxer(), goahttp.RequestDecoder, responseEncoder,
						func(_ context.Context, _ http.ResponseWriter, err error) { t.Error(err) }, nil,
					)
					request := httptest.NewRequest(http.MethodGet, "/pac/report?justification=test&format="+url.QueryEscape(format), nil)
					request.Header.Set("Accept", accept)
					recorder := httptest.NewRecorder()
					reportContentTypeMiddleware(handler).ServeHTTP(recorder, request)
					if recorder.Code != status {
						t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
					}
					mediaType, _, err := mime.ParseMediaType(recorder.Header().Get("Content-Type"))
					if err != nil {
						t.Fatal(err)
					}
					want := "application/json"
					if status == 200 {
						switch format {
						case "csv", " CSV ":
							want = "text/csv"
						case "pdf":
							want = "application/pdf"
						}
						if !bytes.Equal(recorder.Body.Bytes(), content) {
							t.Fatal("report bytes were encoded")
						}
					} else if !json.Valid(recorder.Body.Bytes()) {
						t.Fatalf("non-JSON error: %s", recorder.Body)
					}
					if mediaType != want {
						t.Fatalf("media type %q, want %q", mediaType, want)
					}
				})
			}
		}
	}
}
