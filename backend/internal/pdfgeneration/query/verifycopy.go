package query

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

// Values of PDFVerifyResult.C2paManifestSource.
const (
	manifestSourceEmbedded = "embedded"
	manifestSourceRemote   = "remote"
)

type VerifyContractPdfCopyQry struct {
	DID string
	PDF []byte
}

// VerifyContractPdfCopyHandler verifies a copy of a contract PDF that a caller
// submits (DCS-OR-C2PA-008). The remote manifest of a contract is the manifest
// embedded in the contract PDF this instance stores: GET /c2pa/manifest/{did}
// serves it from there.
type VerifyContractPdfCopyHandler struct {
	Stored *VerifyContractPdfHandler
	Export *ExportContractPdfHandler
}

func (h *VerifyContractPdfCopyHandler) Handle(ctx context.Context, qry VerifyContractPdfCopyQry) (*pdfgen.PDFVerifyResult, error) {
	stored, err := h.Stored.Handle(ctx, VerifyContractPdfQry{DID: qry.DID})
	if err != nil {
		return nil, err
	}
	parties, err := h.parties(ctx, qry.DID)
	if err != nil {
		return nil, err
	}
	storedPDF := func(ctx context.Context) ([]byte, error) {
		pdf, err := h.Export.Handle(ctx, ExportContractPdfQry{DID: qry.DID})
		if err != nil {
			return nil, err
		}
		defer func() { _ = pdf.Close() }()
		return io.ReadAll(pdf)
	}
	return verifyCopy(ctx, h.Stored.PDFCore, h.Stored.Credentials, h.Stored.CredentialStatus, h.Stored.Signatures,
		SignerAnchors{Roots: h.Stored.SignerRoots, Issuers: parties}, qry, parties, stored, storedPDF)
}

// parties returns the DIDs of the DCS instances that are party to the contract.
func (h *VerifyContractPdfCopyHandler) parties(ctx context.Context, did string) ([]string, error) {
	tx, err := h.Stored.DB.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}()
	contract, err := h.Stored.CRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("contract %s: %w", did, err)
	}
	if contract.Responsible == nil {
		return nil, nil
	}
	return contract.Responsible.GetParties(), nil
}

// verifyCopy verifies a copy of the contract PDF this instance stores.
//
// A copy that carries its manifest and is the stored PDF, or an earlier
// revision of it (its bytes start the stored bytes), is verified against its
// own manifest. Any other copy, with or without an embedded manifest, is
// verified against the remote manifest: the manifest of the stored PDF. Its
// XMP reference must name this contract's remote manifest, at this instance or
// at the instance of a party, and it must show exactly the pages and carry the
// payload of the stored PDF. The result is then the verification of the stored
// PDF.
//
// A self-consistent PDF of another contract, or one built under another
// issuer, is therefore never verified against itself.
func verifyCopy(
	ctx context.Context,
	pdfCore *pdfcore.Client,
	credentials *provenance.CredentialVerifier,
	statuses *provenance.CredentialStatusVerifier,
	signatures PDFSignatureValidator,
	anchors SignerAnchors,
	qry VerifyContractPdfCopyQry,
	parties []string,
	stored *pdfgen.PDFVerifyResult,
	storedPDF func(context.Context) ([]byte, error),
) (*pdfgen.PDFVerifyResult, error) {
	lifecycle := ""
	if stored.LifecycleStatus != nil {
		lifecycle = *stored.LifecycleStatus
	}

	original, err := storedPDF(ctx)
	if err != nil {
		return nil, fmt.Errorf("get the contract PDF of %s that holds its remote manifest: %w", qry.DID, err)
	}

	if manifest, err := pdfCore.ExtractManifest(ctx, qry.PDF); err == nil && len(manifest) > 0 && bytes.HasPrefix(original, qry.PDF) {
		result, err := runVerify(ctx, qry.PDF, pdfCore, credentials, statuses, signatures, anchors, lifecycle)
		if err != nil {
			return nil, err
		}
		result.C2paManifestSource = ptrToString(manifestSourceEmbedded)
		return result, nil
	}

	ref, err := pdfCore.ManifestReference(ctx, qry.PDF)
	if err != nil {
		return nil, fmt.Errorf("read the remote manifest reference of the copy of %s: %w", qry.DID, err)
	}
	if !namesThisContractsManifest(ref, qry.DID, parties) {
		return &pdfgen.PDFVerifyResult{
			Match:               false,
			C2paManifestFound:   false,
			C2paSignatureStatus: provenance.CheckNotAvailable,
			C2paSignerStatus:    provenance.CheckNotAvailable,
			VcProofStatus:       provenance.CheckNotAvailable,
			StatusListCheck:     "not_available",
			LifecycleStatus:     stored.LifecycleStatus,
			PdfSignatureStatus:  provenance.CheckNotAvailable,
			Discrepancy:         ptrToString(discrepancyFailed),
		}, nil
	}

	samePages, mismatch, err := pdfCore.MatchContentExact(ctx, qry.PDF, original)
	if err != nil {
		return nil, fmt.Errorf("compare the pages of the copy of %s: %w", qry.DID, err)
	}
	copyPayload, err := pdfCore.ExtractPayload(ctx, qry.PDF)
	if err != nil {
		return nil, fmt.Errorf("read the payload of the copy of %s: %w", qry.DID, err)
	}
	originalPayload, err := pdfCore.ExtractPayload(ctx, original)
	if err != nil {
		return nil, fmt.Errorf("read the payload of the contract PDF of %s: %w", qry.DID, err)
	}

	result := *stored
	result.C2paManifestSource = ptrToString(manifestSourceRemote)
	if !samePages || !bytes.Equal(copyPayload, originalPayload) {
		result.Match = false
		result.Discrepancy = ptrToString(discrepancyHashMismatch)
		log.Printf("pdfgeneration: the copy of %s does not match the contract PDF: %s", qry.DID, mismatch)
	}
	return &result, nil
}

// namesThisContractsManifest reports whether ref is the remote manifest URL of
// contract did: this instance's, or the same path at the host of a party's
// did:web. The last manifest of a contract can be written by either party's
// instance, and each writes its own URL into the copy.
func namesThisContractsManifest(ref, did string, parties []string) bool {
	if ref == "" {
		return false
	}
	if ref == provenance.RemoteManifestURL(did) {
		return true
	}
	u, err := url.Parse(ref)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !strings.HasSuffix(u.Path, "/c2pa/manifest/"+did) {
		return false
	}
	for _, party := range parties {
		host, err := identity.DIDWebToHostname(party)
		if err == nil && strings.EqualFold(u.Host, host) {
			return true
		}
	}
	return false
}
