package event

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

type memoryObjects struct{ blobs map[string][]byte }

func (o *memoryObjects) CreateFile(_ context.Context, data any) (*ipfs.IPFSResult, error) {
	raw, ok := data.([]byte)
	if !ok {
		return nil, fmt.Errorf("expected bytes, got %T", data)
	}
	sum := sha256.Sum256(raw)
	cid := hex.EncodeToString(sum[:])
	o.blobs[cid] = bytes.Clone(raw)
	return o.FetchFile(cid)
}

func (o *memoryObjects) FetchFile(cid string) (*ipfs.IPFSResult, error) {
	raw, ok := o.blobs[cid]
	if !ok {
		return nil, fmt.Errorf("missing object %s", cid)
	}
	result := &ipfs.IPFSResult{Data: bytes.Clone(raw)}
	result.Identifier.Format = "CID"
	result.Identifier.Value = cid
	return result, nil
}

type ecdhAgreement struct{ key *ecdsa.PrivateKey }

func (a ecdhAgreement) DeriveECDH(peer *ecdsa.PublicKey) ([]byte, error) {
	private, err := a.key.ECDH()
	if err != nil {
		return nil, err
	}
	public, err := peer.ECDH()
	if err != nil {
		return nil, err
	}
	return private.ECDH(public)
}

// issuedLifecycleVC is one credential the fake issuer was asked for.
type issuedLifecycleVC struct{ fileHash, status, reason string }

type recordingVCIssuer struct{ issued []issuedLifecycleVC }

func (r *recordingVCIssuer) IssueContractLifecycleVC(_ context.Context, _, fileHash, status, reason, _ string, _ time.Time) (string, json.RawMessage, error) {
	r.issued = append(r.issued, issuedLifecycleVC{fileHash, status, reason})
	vc, _ := json.Marshal(map[string]any{"credentialSubject": map[string]any{"status": status}})
	return "urn:vc:" + status, vc, nil
}

// appendingPDFCore stands in for pdf-core: /payload/extract returns the payload
// the PDF embeds, and an amendment appends a revision naming the lifecycle
// status of the credential it carries, the way pdf-core appends a
// provenance-only update over a signed PDF.
func appendingPDFCore(t *testing.T, embeddedPayload string) *pdfcore.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/payload/extract":
			_, _ = w.Write([]byte(embeddedPayload))
		case "/render/amendment":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if r.FormValue("payload") != embeddedPayload {
				http.Error(w, "the amendment must carry the payload the signed PDF embeds", http.StatusBadRequest)
				return
			}
			var vc struct {
				CredentialSubject struct {
					Status string `json:"status"`
				} `json:"credentialSubject"`
			}
			_ = json.Unmarshal([]byte(r.FormValue("vc")), &vc)
			updated := r.FormValue("pdf") + "\n% lifecycle " + vc.CredentialSubject.Status
			_ = json.NewEncoder(w).Encode(map[string]any{
				"pdf_base64":          base64.StdEncoding.EncodeToString([]byte(updated)),
				"c2pa_sig_structures": []string{},
			})
		case "/c2pa/embed":
			var body struct {
				PDFBase64 string `json:"pdf_base64"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			pdf, _ := base64.StdEncoding.DecodeString(body.PDFBase64)
			_, _ = w.Write(pdf)
		default:
			_, _ = io.Copy(io.Discard, r.Body)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return pdfcore.New(srv.URL, func([]byte) ([]byte, error) { return make([]byte, 64), nil })
}

// signedLifecycleSubscriber stores signedPDF as the PDF of contract did, signed
// and asserting storedState, with the contract itself in contractState.
func signedLifecycleSubscriber(t *testing.T, did, contractState, storedState string, signed bool, signedPDF []byte) (*Subscriber, *recordingVCIssuer, *sqlx.DB) {
	t.Helper()
	database := pgtest.Open(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	store := artifactstore.New(&memoryObjects{blobs: map[string][]byte{}}, &artifactstore.PostgresCEKRepo{DB: database},
		ecdhAgreement{key}, "did:web:dcs-a.example", "did:web:dcs-a.example#agreement", &key.PublicKey)
	cid, err := store.Put(context.Background(), artifactstore.ContractScope(did), signedPDF)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data,pdf_ipfs_cid,pdf_c2pa_state,pdf_payload_hash,pdf_signed)
		VALUES ($1,'did:web:dcs-a.example','Org A',$2,'tpl',1,'{}',$3,$4,'hash-at-signing',$5)`, did, contractState, cid, storedState, signed); err != nil {
		t.Fatal(err)
	}
	issuer := &recordingVCIssuer{}
	return &Subscriber{
		DB:        database,
		Artifacts: store,
		CRepo:     &cwepg.PostgresContractRepo{},
		PDFCore:   appendingPDFCore(t, `{"signed":"payload"}`),
		IssuerDID: "did:web:dcs-a.example",
		VCIssuer:  issuer,
	}, issuer, database
}

func storedContractPDF(t *testing.T, s *Subscriber, database *sqlx.DB, did string) (pdf []byte, c2paState string, signed bool) {
	t.Helper()
	var row struct {
		CID    string `db:"pdf_ipfs_cid"`
		State  string `db:"pdf_c2pa_state"`
		Signed bool   `db:"pdf_signed"`
	}
	if err := database.Get(&row, `SELECT pdf_ipfs_cid, pdf_c2pa_state, pdf_signed FROM contracts WHERE did = $1`, did); err != nil {
		t.Fatal(err)
	}
	pdf, err := s.Artifacts.Get(context.Background(), artifactstore.ContractScope(did), row.CID)
	if err != nil {
		t.Fatal(err)
	}
	return pdf, row.State, row.Signed
}

// DCS-OR-C2PA-003/-004: suspended, terminated and expired arise only after
// signing. Each gets its lifecycle manifest and credential appended to the
// signed PDF as an incremental update, the way the re-anchor is appended
// (ADR-26): the signed bytes stay a prefix, so every signature keeps
// verifying, and the payload the signed PDF embeds is carried unchanged.
func TestRevokedSignedContractAppendsSuspendedAssertionAndVC(t *testing.T) {
	const did = "did:web:dcs-a.example:contract:revoked"
	signedPDF := []byte("%PDF signed revision")
	s, issuer, database := signedLifecycleSubscriber(t, did, "REVOKED", "active", true, signedPDF)
	revocation := minimalCWEEvent{DID: did, OccurredAt: time.Now().UTC(), Reason: "Key compromise of the signatory"}

	if err := s.appendC2PA(context.Background(), revocation); err != nil {
		t.Fatalf("appendC2PA: %v", err)
	}

	pdf, state, signed := storedContractPDF(t, s, database, did)
	if !bytes.HasPrefix(pdf, signedPDF) || !bytes.HasSuffix(pdf, []byte("% lifecycle suspended")) {
		t.Fatalf("the suspended manifest was not appended to the signed bytes: %q", pdf)
	}
	if state != "suspended" || !signed {
		t.Fatalf("stored PDF state = %q signed=%v, want suspended and still signed", state, signed)
	}
	sum := sha256.Sum256(signedPDF)
	want := issuedLifecycleVC{fileHash: hex.EncodeToString(sum[:]), status: "suspended", reason: "Key compromise of the signatory"}
	if len(issuer.issued) != 1 || issuer.issued[0] != want {
		t.Fatalf("issued credentials = %+v, want exactly %+v", issuer.issued, want)
	}

	// A redelivered event finds the PDF already asserting the state.
	if err := s.appendC2PA(context.Background(), revocation); err != nil {
		t.Fatalf("appendC2PA on redelivery: %v", err)
	}
	if again, _, _ := storedContractPDF(t, s, database, did); !bytes.Equal(again, pdf) || len(issuer.issued) != 1 {
		t.Fatal("a redelivered event appended a second manifest")
	}
}

// A contract terminated or expired before anyone signed has an unsigned PDF
// that was rendered in that state already; there is nothing to append.
func TestUnsignedFrozenContractGetsNoAppendedManifest(t *testing.T) {
	const did = "did:web:dcs-a.example:contract:unsigned"
	unsigned := []byte("%PDF unsigned terminated render")
	s, issuer, database := signedLifecycleSubscriber(t, did, "EXPIRED", "terminated", false, unsigned)

	if err := s.appendC2PA(context.Background(), minimalCWEEvent{DID: did, OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatalf("appendC2PA: %v", err)
	}
	if pdf, state, _ := storedContractPDF(t, s, database, did); !bytes.Equal(pdf, unsigned) || state != "terminated" || len(issuer.issued) != 0 {
		t.Fatalf("an unsigned frozen PDF was changed: state %q, %d credentials", state, len(issuer.issued))
	}
}

// A lost revoke, terminate or expiry event is not redelivered, so the retry
// sweep selects a signed PDF that does not yet assert its contract's
// post-signing state, and stops selecting it once the manifest is appended.
func TestRetrySweepSelectsASignedPDFMissingItsLifecycleManifest(t *testing.T) {
	const did = "did:web:dcs-a.example:contract:terminated"
	s, issuer, database := signedLifecycleSubscriber(t, did, "TERMINATED", "active", true, []byte("%PDF signed revision"))
	if _, err := database.Exec(`INSERT INTO contract_signatures (contract_did, status, signature_bytes, signer_did, credential_type) VALUES ($1,'SIGNED','\x00','did:jwk:signer','AES')`, did); err != nil {
		t.Fatal(err)
	}

	selected := func() bool {
		tx, err := database.BeginTxx(context.Background(), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		dids, err := s.CRepo.ReadDIDsNeedingRegeneration(context.Background(), tx, 10, nil)
		if err != nil {
			t.Fatal(err)
		}
		return len(dids) == 1 && dids[0] == did
	}
	if !selected() {
		t.Fatal("the sweep does not select a signed PDF missing its terminated manifest")
	}
	if err := s.appendC2PA(context.Background(), minimalCWEEvent{DID: did, OccurredAt: time.Now().UTC()}); err != nil {
		t.Fatalf("appendC2PA: %v", err)
	}
	if selected() {
		t.Fatal("the sweep still selects the contract after its manifest was appended")
	}
	// The sweep has no event to take a reason from; the assertion still names one.
	if len(issuer.issued) != 1 || issuer.issued[0].reason != provenance.DefaultLifecycleReason("terminated") {
		t.Fatalf("issued %+v, want the terminated credential with the default reason", issuer.issued)
	}
}
