package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	goahttp "goa.design/goa/v3/http"

	pdfgenerationsvr "digital-contracting-service/gen/http/pdf_generation/server"
	signaturemanagementsvr "digital-contracting-service/gen/http/signature_management/server"
	pdfgeneration "digital-contracting-service/gen/pdf_generation"
	signaturemanagement "digital-contracting-service/gen/signature_management"
)

// DCS-IR-CI-02: binary artifacts are served as application/pdf, over GET.
// The generated handlers stream the body without a Content-Type of their own,
// so the media type comes from net/http, which detects "%PDF-".
func TestPDFEndpointsServeApplicationPDF(t *testing.T) {
	pdf := []byte("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n1 0 obj\n<<>>\nendobj\n")
	body := func() io.ReadCloser { return io.NopCloser(bytes.NewReader(pdf)) }
	noErr := func(_ context.Context, _ http.ResponseWriter, err error) { t.Error(err) }

	mux := goahttp.NewMuxer()
	pdfgenerationsvr.MountExportContractPdfHandler(mux, pdfgenerationsvr.NewExportContractPdfHandler(
		func(context.Context, any) (any, error) {
			return &pdfgeneration.ExportContractPdfResponseData{Body: body()}, nil
		}, mux, goahttp.RequestDecoder, responseEncoder, noErr, nil))
	pdfgenerationsvr.MountExportTemplatePdfHandler(mux, pdfgenerationsvr.NewExportTemplatePdfHandler(
		func(context.Context, any) (any, error) {
			return &pdfgeneration.ExportTemplatePdfResponseData{Body: body()}, nil
		}, mux, goahttp.RequestDecoder, responseEncoder, noErr, nil))
	signaturemanagementsvr.MountSignatureRequestDocumentHandler(mux, signaturemanagementsvr.NewSignatureRequestDocumentHandler(
		func(context.Context, any) (any, error) {
			return &signaturemanagement.SignatureRequestDocumentResponseData{Body: body()}, nil
		}, mux, goahttp.RequestDecoder, responseEncoder, noErr, nil))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	for _, path := range []string{
		"/pdf/export/contract/did:web:a.example:contract:1",
		"/pdf/export/template/did:web:a.example:template:1",
		"/signature/request/ceremony-1/document",
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || !bytes.Equal(got, pdf) {
			t.Fatalf("GET %s = %d, body changed: %t", path, resp.StatusCode, !bytes.Equal(got, pdf))
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
			t.Fatalf("GET %s Content-Type = %q, want application/pdf", path, ct)
		}
	}
}
