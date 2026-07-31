package command

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/digitorus/pkcs7"
	"github.com/stretchr/testify/require"
)

// TestCredentialTypeAtLeast locks the SES < AES < QES ranking the prepare-time
// fail-fast check and the submit-time level gate both rely on (SM-01, ADR-20).
func TestCredentialTypeAtLeast(t *testing.T) {
	require.True(t, credentialTypeAtLeast("QES", "AES"))
	require.True(t, credentialTypeAtLeast("QES", "QES"))
	require.True(t, credentialTypeAtLeast("AES", "AES"))
	require.False(t, credentialTypeAtLeast("AES", "QES"))
	require.False(t, credentialTypeAtLeast("", "AES"))
	require.True(t, credentialTypeAtLeast("aes", "AES"), "comparison is case-insensitive")
}

func TestPidGivenFamilyName(t *testing.T) {
	given, family := pidGivenFamilyName([]byte(`{"given_name":"Jane","family_name":"Doe"}`))
	require.Equal(t, "Jane", given)
	require.Equal(t, "Doe", family)

	given, family = pidGivenFamilyName([]byte(`{"givenName":"Jane","familyName":"Doe"}`))
	require.Equal(t, "Jane", given)
	require.Equal(t, "Doe", family)

	given, family = pidGivenFamilyName(nil)
	require.Equal(t, "", given)
	require.Equal(t, "", family)

	given, family = pidGivenFamilyName([]byte(`not json`))
	require.Equal(t, "", given)
	require.Equal(t, "", family)
}

// TestNamesMatch proves the sole-control name gate (ADR-20 item 4): the PID
// and the certificate must agree on both given name and surname, tolerant of
// case and whitespace, and an absent certificate name is never a match by
// omission — a certificate that carries no name at all must never pass.
func TestNamesMatch(t *testing.T) {
	require.True(t, namesMatch("Jane", "Doe", "jane", "  DOE  "), "case/whitespace tolerant")
	require.False(t, namesMatch("Jane", "Doe", "Jane", "Smith"), "surname mismatch")
	require.False(t, namesMatch("Jane", "Doe", "John", "Doe"), "given name mismatch")
	require.False(t, namesMatch("Jane", "Doe", "", ""), "no certificate name is never a match")
	require.False(t, namesMatch("", "", "Jane", "Doe"), "no PID name is never a match")
}

// TestSignerCertificateFromIncrementalUpdate proves the sole-control gate's
// certificate identity extraction (ADR-20 item 4) end to end: mint a leaf
// certificate carrying GIVENNAME/SURNAME RDNs the same way
// testWallet/dcs_wallet/signer.py's ensure_signing_material does, wrap it in
// a detached CMS SignedData the same way DSS's signDocument would, embed
// that as a PDF hex-string /Contents value (with the same trailing zero
// padding a real /Contents placeholder carries), and confirm the whole chain
// - byte-prefix delta, hex decode, CMS parse, GetOnlySigner, RDN read -
// recovers the identity without going through DSS's validation report at
// all (dss/client.go's report never carries these fields reliably; see
// signerCertificateFromIncrementalUpdate's doc comment).
func TestSignerCertificateFromIncrementalUpdate(t *testing.T) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "DCS Wallet Dev Signing CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(3650 * 24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	caCert, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "DCS Signatory Instance B Signatory",
			ExtraNames: []pkix.AttributeTypeAndValue{
				{Type: oidGivenName, Value: "Instance B Signatory"},
				{Type: oidSurname, Value: "BDD-Testperson"},
			},
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(825 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	require.NoError(t, err)
	leafCert, err := x509.ParseCertificate(leafDER)
	require.NoError(t, err)

	signedData, err := pkcs7.NewSignedData([]byte("the data to be signed"))
	require.NoError(t, err)
	require.NoError(t, signedData.AddSigner(leafCert, leafKey, pkcs7.SignerInfoConfig{}))
	signedData.Detach()
	cms, err := signedData.Finish()
	require.NoError(t, err)

	// /Contents is pre-allocated at prepare time to a fixed length and DSS's
	// actual CMS content rarely fills it exactly - the remainder pads with
	// zero bytes, which a correct DER parse must tolerate.
	padded := make([]byte, len(cms)+64)
	copy(padded, cms)
	preparedPDF := []byte("%PDF-1.7 fake prepared document\n")
	delta := []byte("12 0 obj\n<< /Type /Sig /Filter /Adobe.PPKLite /Contents <" +
		hex.EncodeToString(padded) + "> /ByteRange [0 1 2 3] >>\nendobj\n")
	signedPDF := append(append([]byte{}, preparedPDF...), delta...)

	cert, err := signerCertificateFromIncrementalUpdate(signedPDF, preparedPDF)
	require.NoError(t, err)
	given, surname := certGivenSurname(cert)
	require.Equal(t, "Instance B Signatory", given)
	require.Equal(t, "BDD-Testperson", surname)
}

// TestJWSPayloadAndHeaderRoundTrip proves the nonce-binding and payload-pin
// extraction helpers correctly recover a custom protected-header claim and
// the raw payload from a compact JWS, without needing a real signature — the
// signature itself is DSS's job; these just decode what DSS already
// validated (ADR-20 item 1/2).
func TestJWSPayloadAndHeaderRoundTrip(t *testing.T) {
	header := map[string]any{"alg": "ES256", "nonce": "abc-123"}
	headerBytes, err := json.Marshal(header)
	require.NoError(t, err)
	payload := []byte(`{"dcs:contractDid":"did:web:example#1"}`)

	compact := base64.RawURLEncoding.EncodeToString(headerBytes) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString([]byte("sig"))

	nonce, err := jwsProtectedHeaderClaim(compact, "nonce")
	require.NoError(t, err)
	require.Equal(t, "abc-123", nonce)

	missing, err := jwsProtectedHeaderClaim(compact, "not_present")
	require.NoError(t, err)
	require.Equal(t, "", missing)

	got, err := jwsPayloadBytes(compact)
	require.NoError(t, err)
	require.Equal(t, payload, got)

	_, err = jwsPayloadBytes("not-a-jws")
	require.Error(t, err)
	_, err = jwsProtectedHeaderClaim("not-a-jws", "nonce")
	require.Error(t, err)
}
