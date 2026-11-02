package query

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/signingmanagement/db"
)

// DCS-FR-SM-07, DCS-FR-SM-17, DCS-FR-SM-24: retrieve_by_id reports each
// declared field, so a second signer on one instance still finds an unsigned
// field after the first signature, and a revoked signature leaves its field
// unsigned.
func TestRetrieveByIDReportsEachSignatureFieldStatus(t *testing.T) {
	const local, peer = "did:web:dcs-a.example", "did:web:dcs-b.example"
	signedAt := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
	one, two, three := "SignerOne", "SignerTwo", "SignerThree"
	records := []db.SignatureRecord{
		{SignerDID: "did:jwk:one", Status: "SIGNED", FieldName: &one, SignedAt: &signedAt},
		{SignerDID: "did:jwk:three", Status: "REVOKED", FieldName: &three, SignedAt: &signedAt},
	}
	fields := signatureFieldStatuses(
		[]string{one, two, three, peer},
		&db.Responsible{Creator: local, Counterparty: peer}, local,
		records, []db.PeerSigner{{PeerDID: peer, ReceivedAt: signedAt}},
	)

	require.Len(t, fields, 4)
	require.True(t, fields[0].Signed)
	require.Equal(t, "did:jwk:one", *fields[0].SignerDID)
	require.False(t, fields[1].Signed, "the second signer's field reads signed")
	require.False(t, fields[2].Signed, "a revoked signature reads signed")
	require.True(t, fields[3].Remote)
	require.True(t, fields[3].Signed)
	require.False(t, fields[0].Remote)
}
