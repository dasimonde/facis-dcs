package service

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/middleware"
)

// UC-IF-29 (DCS-IR-CI-02): an archived evidence document is served as a file:
// a PDF as application/pdf, any other document as application/octet-stream,
// so a browser never renders an uploaded document from the API origin.
func TestArchiveEvidenceIsServedAsAFile(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:dcs.example:contract:evidence-file"
	seedArchivedContract(t, db, did, "SIGNED")
	svc := archiveTestService(t, db)
	artifacts := newMemoryArtifacts()
	svc.Artifacts = artifacts
	store := func(name, mediaType string, content []byte) string {
		res, err := svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{
			Kind: "evidence", Did: did, Name: &name, MediaType: &mediaType, Content: content,
		})
		require.NoError(t, err)
		return *res.EvidenceID
	}
	pdf := store("note.pdf", "application/pdf", []byte("%PDF-1.7 note"))
	page := store("page.html", "text/html", []byte("<script>alert(1)</script>"))

	res, body, err := svc.EvidenceContent(archiveManagerContext(), &contractstoragearchive.EvidenceContentPayload{Did: did, EvidenceID: pdf})
	require.NoError(t, err)
	require.Equal(t, "application/pdf", res.ContentType)
	content, _ := io.ReadAll(body)
	require.Equal(t, "%PDF-1.7 note", string(content))

	res, _, err = svc.EvidenceContent(archiveManagerContext(), &contractstoragearchive.EvidenceContentPayload{Did: did, EvidenceID: page})
	require.NoError(t, err)
	require.Equal(t, "application/octet-stream", res.ContentType)

	_, _, err = svc.EvidenceContent(archiveManagerContext(), &contractstoragearchive.EvidenceContentPayload{Did: did, EvidenceID: "unknown"})
	require.Equal(t, "not_found", errorName(err))
	artifacts.shredded[did] = true
	_, _, err = svc.EvidenceContent(archiveManagerContext(), &contractstoragearchive.EvidenceContentPayload{Did: did, EvidenceID: pdf})
	require.Equal(t, "not_found", errorName(err))
}

// UC-IF-29 (DCS-IR-CI-02): the document prepared for signing is served as a
// PDF to the session that started the ceremony, and to no other session.
func TestThePreparedDocumentIsServedToItsSession(t *testing.T) {
	_, service := ceremonyTestService(t)
	own := middleware.InjectAuthContext(context.Background(), []string{"Contract Signer"}, "did:jwk:org-y-session", ceremonyTestOrg)

	body, err := service.PreparedDocument(own, &signaturemanagement.PreparedDocumentPayload{CeremonyID: ceremonyTestID})
	require.NoError(t, err)
	content, _ := io.ReadAll(body)
	require.Equal(t, "%PDF-prepared", string(content))

	other := middleware.InjectAuthContext(context.Background(), []string{"Contract Signer"}, "did:jwk:org-y-other-session", ceremonyTestOrg)
	_, err = service.PreparedDocument(other, &signaturemanagement.PreparedDocumentPayload{CeremonyID: ceremonyTestID})
	require.Equal(t, "forbidden", errorName(err))
}
