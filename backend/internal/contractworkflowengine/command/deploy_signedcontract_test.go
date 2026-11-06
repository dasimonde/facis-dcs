package command

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/contractworkflowengine/db"
	db2 "digital-contracting-service/internal/dcstodcs/db"
)

// DCS-FR-SM-12: the deployment trigger carries the signed contract: the signed
// PDF with its CID and hash, and every signature this instance holds,
// including the counterparty's cross-instance signature.
func TestSignedContractPayloadCarriesThePDFAndEverySignature(t *testing.T) {
	pdf := []byte("%PDF-1.7 signed")
	signedAt := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	payload := signedContractPayload("bafy-signed", pdf,
		[]db.SignedSignature{{SignerDID: "did:jwk:signer", CredentialType: "AES", FieldName: peerA, SignedAt: signedAt, PDFHash: "abc", JAdESSignature: "jades-a"}},
		&db2.SyncSignature{FromPeerDID: peerB, JadesSignature: "jades-b", ReceivedAt: signedAt})

	sum := sha256.Sum256(pdf)
	signed := payload["dcs:signedContract"].(map[string]any)
	require.Equal(t, "bafy-signed", signed["dcs:cid"])
	require.Equal(t, "sha256:"+hex.EncodeToString(sum[:]), signed["dcs:sha256"])
	content, err := base64.StdEncoding.DecodeString(signed["dcs:content"].(string))
	require.NoError(t, err)
	require.Equal(t, pdf, content)

	signatures := payload["dcs:signatures"].([]any)
	require.Len(t, signatures, 2)
	own := signatures[0].(map[string]any)
	require.Equal(t, "did:jwk:signer", own["dcs:signer"])
	require.Equal(t, "AES", own["dcs:credentialType"])
	require.Equal(t, "jades-a", own["dcs:jadesSignature"])
	require.Equal(t, "sha256:abc", own["dcs:pdfHash"])
	require.Equal(t, "jades-b", signatures[1].(map[string]any)["dcs:jadesSignature"])

	// The content hash the target verifies covers the signed contract too.
	withPDF, err := hashDeploymentPayload(map[string]any{"dcs:signedContract": signed})
	require.NoError(t, err)
	without, err := hashDeploymentPayload(map[string]any{})
	require.NoError(t, err)
	require.NotEqual(t, withPDF, without)
}
