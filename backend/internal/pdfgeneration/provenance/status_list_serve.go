package provenance

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/hsm"

	"github.com/jmoiron/sqlx"
)

// StatusListMediaType is what a token status list is served and detected as
// (status.SelectMechanismFromResponse routes this content type, and only this
// one, to the IETF token handler).
const StatusListMediaType = "application/statuslist+jwt"

// StatusListPath is the origin-root path a served list answers under, mirroring
// did.json: a verifier reaches it without knowing this deployment's API prefix,
// and the ingress routes it to the backend the same way.
const StatusListPath = "/status-list/"

// StatusListSigner builds and signs the status list this deployment serves
// (ADR-34): an IETF Token Status List in a JWT, ES256, `typ: statuslist+jwt`,
// carrying the certificate chain that publishes the signing key.
//
// The chain is the one the hsm-provision job already issues for the C2PA claim
// signature (c2pa-x5chain.pem), bound to the same PKCS#11 key. Both are this
// deployment asserting something about a document it produced, so a second
// certificate for the second assertion would add a key ceremony without adding
// a distinction — and a verifier that trusts one already trusts the other.
type StatusListSigner struct {
	// Issuer is the `iss` the token names, and it must be the identity that
	// issued the credentials this list governs — the deployment's ISSUER_DID,
	// the same value NewLocalVCIssuer writes into every credential's `issuer`.
	//
	// Not the public origin, though the leaf carries that too: a verifier binds
	// a list to the credential by comparing the two identifiers as strings
	// (status.RequireCredentialIssuer, ADR-34/-35). Naming the origin here while
	// the credentials name the DID describes one deployment two ways, and every
	// revocation check of our own credentials then reports "signed by an issuer
	// other than the one it names" — which a caller reads as an unknown, not as
	// a configuration error.
	//
	// The chain's leaf must carry whichever identity this names, or the list is
	// refused as coming from an unidentified issuer
	// (sdjwt.VerificationKeyFromX5C); c2pa-cert-provision.sh puts the DID, the
	// issuer URL and the hostname on it, and leafIdentifiesIssuer accepts any.
	Issuer string

	// ListURI resolves a list id to the absolute URI a credential names. It is
	// emitted verbatim as `sub`, which the verifier compares against the URI it
	// was sent to and refuses on any difference.
	ListURI func(listID int) string

	// ListID is the single list this deployment serves. A request for any other
	// id is a 404: answering it with this list's bits would hand a verifier one
	// contract's revocation state under another list's name.
	ListID int

	// Chain is the x5c header value: DER certificates, leaf first (RFC 7515 §4.7).
	Chain []string

	// Signer holds the private half of the leaf's key, in the PKCS#11 token.
	Signer crypto.Signer

	// Revocations supplies the set bits.
	Revocations StatusListRevocations

	// Size is the number of entries the bitstring covers.
	Size int

	// Now is the clock the `iat` claim is stamped from; nil means time.Now.
	Now func() time.Time
}

// LoadX5Chain reads a PEM certificate chain into the base64 DER entries an x5c
// header carries, leaf first.
//
// The last entry is the root a verifier has to hold as an anchor to accept
// anything signed under this chain — which is why StatusListRoot exists beside
// this, rather than every caller re-deriving "the last one" and one of them
// getting it wrong.
func LoadX5Chain(path string) ([]string, error) {
	pemBytes, err := os.ReadFile(path) //nolint:gosec // operator-configured path to this deployment's own certificate chain
	if err != nil {
		return nil, fmt.Errorf("read certificate chain %s: %w", path, err)
	}
	certs, err := parseCertificateChain(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("certificate chain %s: %w", path, err)
	}
	chain := make([]string, 0, len(certs))
	for _, cert := range certs {
		chain = append(chain, base64.StdEncoding.EncodeToString(cert.Raw))
	}
	return chain, nil
}

// StatusListRoot returns the trust anchor of the chain at path: the last
// certificate, which every signature under this chain has to reach.
//
// A deployment verifies its own status list through exactly the path it
// verifies anyone else's — chain against configured anchors, leaf against the
// issuer the token names — so the anchor of its own chain has to be among those
// anchors. It is generated per install (the provisioning job mints it), so it
// cannot be a committed fixture and is read back from the chain instead.
func StatusListRoot(path string) (*x509.Certificate, error) {
	pemBytes, err := os.ReadFile(path) //nolint:gosec // operator-configured path to this deployment's own certificate chain
	if err != nil {
		return nil, fmt.Errorf("read certificate chain %s: %w", path, err)
	}
	certs, err := parseCertificateChain(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("certificate chain %s: %w", path, err)
	}
	root := certs[len(certs)-1]
	if !root.IsCA {
		return nil, fmt.Errorf("certificate chain %s does not end in a CA certificate, so it reaches no trust anchor", path)
	}
	return root, nil
}

func parseCertificateChain(pemBytes []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := pemBytes
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("holds no certificate")
	}
	return certs, nil
}

// Token returns the signed status list for this deployment's list.
func (s *StatusListSigner) Token(ctx context.Context) ([]byte, error) {
	if s == nil || s.Signer == nil {
		return nil, fmt.Errorf("status list signer is not configured")
	}
	if len(s.Chain) == 0 {
		return nil, fmt.Errorf("status list signer has no certificate chain: a verifier could not resolve the key")
	}
	if strings.TrimSpace(s.Issuer) == "" || s.ListURI == nil {
		return nil, fmt.Errorf("status list signer has no issuer or list URI")
	}

	bitstring, err := s.bitstring(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now()
	}

	header := map[string]any{
		"alg": "ES256",
		"typ": "statuslist+jwt",
		"x5c": s.Chain,
	}
	// One bit per entry, zlib-compressed and base64url-encoded: the encoding the
	// IETF Token Status List defines and the one the verifier's IETF handler
	// decodes (codec.ZLIBDecompressLimited, codec.LSBFirst).
	payload := map[string]any{
		"iss": s.Issuer,
		"sub": s.ListURI(s.ListID),
		"iat": now.Unix(),
		"status_list": map[string]any{
			"bits": 1,
			"lst":  base64.RawURLEncoding.EncodeToString(deflate(bitstring)),
		},
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return nil, fmt.Errorf("marshal status list header: %w", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal status list payload: %w", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(payloadJSON)
	signature, err := hsm.SignES256(s.Signer, []byte(signingInput))
	if err != nil {
		return nil, fmt.Errorf("sign status list: %w", err)
	}
	return []byte(signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)), nil
}

// bitstring renders the list's revoked entries as one bit each, LSB-first —
// bit N at bit (N%8) of byte N/8.
func (s *StatusListSigner) bitstring(ctx context.Context) ([]byte, error) {
	size := s.Size
	if size <= 0 {
		size = ListSize
	}
	bits := make([]byte, (size+7)/8)
	if s.Revocations == nil {
		return nil, fmt.Errorf("status list signer has no revocation store: it would serve every entry as valid")
	}
	indices, err := s.Revocations.RevokedIndices(ctx, s.ListID)
	if err != nil {
		return nil, fmt.Errorf("read revoked entries of status list %d: %w", s.ListID, err)
	}
	for _, index := range indices {
		if int(index) >= size {
			// The allocator refuses an index past the list's registered size, so
			// one here means the list was shrunk under credentials that already
			// name entries in it. Serving a shorter list would answer "valid"
			// for exactly those, which is the direction that must not fail.
			return nil, fmt.Errorf(
				"status list %d holds entry %d but is served with only %d entries: credentials naming it would read as valid",
				s.ListID, index, size)
		}
		bits[index/8] |= 1 << (index % 8)
	}
	return bits, nil
}

func deflate(raw []byte) []byte {
	var out bytes.Buffer
	w := zlib.NewWriter(&out)
	// zlib.Writer only fails through the underlying writer, and a bytes.Buffer
	// has no failure mode.
	_, _ = w.Write(raw)
	_ = w.Close()
	return out.Bytes()
}

// StatusListHandler serves the token at StatusListPath.
//
// It is mounted outside the generated API surface, at the origin root, because
// the media type is the routing signal a verifier uses: Goa's response encoder
// negotiates a content type it recognises and would label this
// application/json, at which point a conformant verifier stops treating it as a
// status list at all.
func StatusListHandler(signer *StatusListSigner) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		requested, err := strconv.Atoi(strings.Trim(strings.TrimPrefix(r.URL.Path, StatusListPath), "/"))
		if err != nil || requested != signer.ListID {
			http.Error(w, "no such status list", http.StatusNotFound)
			return
		}

		token, err := signer.Token(r.Context())
		if err != nil {
			http.Error(w, "status list unavailable", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", StatusListMediaType)
		// A revocation has to be visible to the next verifier that asks; a
		// cached copy is a list that says "valid" for as long as the cache holds.
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(token)
	})
}

// PostgresStatusListRevocations is the revocation half of status_list_entries
// (migration 20260737): the same row that records which entry a subject owns
// records whether its bit is set, so the two cannot disagree.
type PostgresStatusListRevocations struct {
	DB *sqlx.DB
}

func (s *PostgresStatusListRevocations) Revoke(ctx context.Context, subjectID string) error {
	subjectID = strings.TrimSpace(subjectID)
	if subjectID == "" {
		return fmt.Errorf("status list revocation needs a subject id")
	}
	result, err := s.DB.ExecContext(ctx,
		`UPDATE status_list_entries
            SET revoked_at = CURRENT_TIMESTAMP
          WHERE subject_id = $1 AND revoked_at IS NULL`, subjectID)
	if err != nil {
		return fmt.Errorf("revoke status list entry of %s: %w", subjectID, err)
	}
	if affected, err := result.RowsAffected(); err == nil && affected == 0 {
		// Either the subject is already revoked, or it holds no entry at all.
		// The second is a credential advertising an index nothing will ever set,
		// so it is separated out rather than passed over as idempotence.
		var exists bool
		if err := s.DB.GetContext(ctx, &exists,
			`SELECT EXISTS (SELECT 1 FROM status_list_entries WHERE subject_id = $1)`, subjectID); err != nil {
			return fmt.Errorf("confirm status list entry of %s: %w", subjectID, err)
		}
		if !exists {
			return fmt.Errorf("%s holds no status list entry to revoke", subjectID)
		}
	}
	return nil
}

func (s *PostgresStatusListRevocations) RevokedIndices(ctx context.Context, listID int) ([]uint32, error) {
	var rows []int64
	if err := s.DB.SelectContext(ctx, &rows,
		`SELECT entry_index FROM status_list_entries
          WHERE list_id = $1 AND revoked_at IS NOT NULL`, listID); err != nil {
		return nil, err
	}
	indices := make([]uint32, 0, len(rows))
	for _, row := range rows {
		indices = append(indices, uint32(row))
	}
	return indices, nil
}
