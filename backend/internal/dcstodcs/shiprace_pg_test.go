package dcstodcs

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	goahttp "goa.design/goa/v3/http"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	dcstodcssvr "digital-contracting-service/gen/http/dcs_to_dcs/server"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/ecutil"
	"digital-contracting-service/internal/base/federation"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

// memoryObjects is an in-memory object store keyed by a counter CID.
type memoryObjects struct {
	mu    sync.Mutex
	blobs map[string][]byte
}

func (m *memoryObjects) CreateFile(_ context.Context, data any) (*ipfs.IPFSResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cid := fmt.Sprintf("cid-%d", len(m.blobs))
	m.blobs[cid] = data.([]byte)
	res := &ipfs.IPFSResult{}
	res.Identifier.Value = cid
	return res, nil
}

func (m *memoryObjects) FetchFile(cid string) (*ipfs.IPFSResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &ipfs.IPFSResult{Data: m.blobs[cid]}, nil
}

type softwareKeyAgreement struct{ priv *ecdsa.PrivateKey }

func (k softwareKeyAgreement) DeriveECDH(peerPub *ecdsa.PublicKey) ([]byte, error) {
	own, err := k.priv.ECDH()
	if err != nil {
		return nil, err
	}
	peer, err := peerPub.ECDH()
	if err != nil {
		return nil, err
	}
	return own.ECDH(peer)
}

// shipPeer is the counterparty instance: its did.json, its agreement
// credential (or none, when refuseCredential is set), the policy endpoint and
// its post_pdf endpoint. It counts the posts it accepted and the most ship
// attempts it saw in flight at once at the credential fetch, which every
// attempt that reaches the trust gate makes.
type shipPeer struct {
	dcstodcs.Service
	dcstodcs.Auther
	did              string
	url              string
	refuseCredential bool
	refusal          error
	posts            atomic.Int32
	inFlight         atomic.Int32
	maxInFlight      atomic.Int32
}

func (p *shipPeer) PostPdf(context.Context, *dcstodcs.DCSToDCSContractPdfRequest) (*dcstodcs.DCSToDCSContractPdfResponse, error) {
	p.posts.Add(1)
	if p.refusal != nil {
		return nil, p.refusal
	}
	return &dcstodcs.DCSToDCSContractPdfResponse{}, nil
}

func ecJWK(key *ecdsa.PrivateKey) map[string]any {
	return map[string]any{"kty": "EC", "crv": "P-256", "x": ecutil.MustJWKX(ecutil.AsPublic(key)), "y": ecutil.MustJWKY(ecutil.AsPublic(key))}
}

func startShipPeer(t *testing.T, refuseCredential bool) *shipPeer {
	t.Helper()
	vcKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	agreementKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	p := &shipPeer{refuseCredential: refuseCredential}
	goaMux := goahttp.NewMuxer()
	dcstodcssvr.Mount(goaMux, dcstodcssvr.New(dcstodcs.NewEndpoints(p), goaMux,
		goahttp.RequestDecoder, goahttp.ResponseEncoder, nil, goahttp.NewErrorResponse))
	mux := http.NewServeMux()
	mux.Handle("/api/", http.StripPrefix("/api", goaMux))
	mux.HandleFunc("/pdp", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("/.well-known/did.json", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": p.did,
			"verificationMethod": []map[string]any{
				{"id": p.did + "#dcs-vc", "type": "JsonWebKey2020", "controller": p.did, "publicKeyJwk": ecJWK(vcKey)},
				{"id": p.did + "#dcs-ecdh", "type": "JsonWebKey2020", "controller": p.did, "publicKeyJwk": ecJWK(agreementKey)},
			},
			"assertionMethod": []any{"#dcs-vc"},
			"keyAgreement":    []any{"#dcs-ecdh"},
		})
	})
	mux.HandleFunc("/.well-known/dcs-agreement-credential.json", func(w http.ResponseWriter, r *http.Request) {
		n := p.inFlight.Add(1)
		for {
			if seen := p.maxInFlight.Load(); n <= seen || p.maxInFlight.CompareAndSwap(seen, n) {
				break
			}
		}
		// Long enough for a second attempt started at the same time to arrive
		// while this one is still in flight.
		time.Sleep(300 * time.Millisecond)
		p.inFlight.Add(-1)
		if p.refuseCredential {
			http.NotFound(w, r)
			return
		}
		credential, err := federation.BuildAgreementCredential(r.Context(), provenance.NewHSMVCSigner(vcKey, "dcs-vc"),
			p.did, "https://rules.example/federation", time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(credential)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	t.Setenv("DCS_API_PATH", "/api")
	p.url = server.URL
	p.did = "did:web:" + strings.ReplaceAll(strings.TrimPrefix(server.URL, "http://"), ":", "%3A")
	return p
}

// shipFixture stores an offered contract with this instance as creator and
// the peer as counterparty, its PDF already stored, and returns the
// synchronizer of this instance.
func shipFixture(t *testing.T, db *sqlx.DB, peer *shipPeer, did string) *DCSToDCSSynchronizer {
	t.Helper()
	ctx := context.Background()
	const localHost = "dcs-a.example"
	document := eraserTestDIDDocument(t, localHost)
	localDID := "did:web:" + localHost
	agreementKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	artifacts := artifactstore.New(&memoryObjects{blobs: map[string][]byte{}}, &artifactstore.PostgresCEKRepo{DB: db},
		softwareKeyAgreement{priv: agreementKey}, localDID, localDID+"#dcs-ecdh", &agreementKey.PublicKey)
	cid, err := artifacts.Put(ctx, artifactstore.ContractScope(did), []byte("%PDF-1.7 offer"))
	require.NoError(t, err)

	responsible, err := json.Marshal(map[string]any{"creator": localDID, "counterparty": peer.did})
	require.NoError(t, err)
	_, err = db.Exec(`
		INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, contract_data, responsible, pdf_ipfs_cid)
		VALUES ($1, $2, $2, 'OFFERED', 'did:web:tpl', 1, '{}', $3, $4)`, did, localDID, string(responsible), cid)
	require.NoError(t, err)

	return &DCSToDCSSynchronizer{
		DB:          db,
		CRepo:       &cwepg.PostgresContractRepo{},
		SRepo:       syncpg.PostgresSyncRepository{},
		Artifacts:   artifacts,
		DIDDocument: *document,
		TrustGate:   TrustGate{PDPURL: peer.url + "/pdp"},
	}
}

// shipTwiceAtOnce starts two ship attempts for the contract at the same time,
// as the offer event and the PDF-regenerated event of one offer do.
func shipTwiceAtOnce(s *DCSToDCSSynchronizer, did string) [2]error {
	var errs [2]error
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = s.shipContractPDF(context.Background(), did)
		}()
	}
	wg.Wait()
	return errs
}

// ADR-19 AC5/AC6: two ship attempts of one offer towards a peer the trust gate
// refuses run one after the other and raise one incident between them.
func TestConcurrentShipsOfARefusedOfferRaiseOneIncident(t *testing.T) {
	db := pgtest.Open(t)
	peer := startShipPeer(t, true)
	did := "ship-race-refused-" + strings.ToLower(rand.Text()[:8])
	s := shipFixture(t, db, peer, did)

	for _, err := range shipTwiceAtOnce(s, did) {
		require.ErrorContains(t, err, "agreement credential")
	}

	var incidents, retryEntries int
	require.NoError(t, db.Get(&incidents, `SELECT count(*) FROM outbox_events WHERE event_type = 'PAC_TRUST_GATE_DENIAL' AND did = $1`, did))
	require.NoError(t, db.Get(&retryEntries, `SELECT count(*) FROM sync_fails WHERE did = $1`, did))
	require.Equal(t, 1, incidents)
	require.Equal(t, 1, retryEntries)
	require.Zero(t, peer.posts.Load())
	require.EqualValues(t, 1, peer.maxInFlight.Load(), "two ship attempts of one contract ran at the same time")
}

// Two ship attempts of one offer towards a trusted peer post it once; a later
// attempt for the same revision posts nothing.
func TestConcurrentShipsOfAnAcceptedOfferPostItOnce(t *testing.T) {
	db := pgtest.Open(t)
	peer := startShipPeer(t, false)
	did := "ship-race-accepted-" + strings.ToLower(rand.Text()[:8])
	s := shipFixture(t, db, peer, did)

	for _, err := range shipTwiceAtOnce(s, did) {
		require.NoError(t, err)
	}
	require.NoError(t, s.shipContractPDF(context.Background(), did))

	require.EqualValues(t, 1, peer.posts.Load(), "the peer received the same revision more than once")
	var sent int
	require.NoError(t, db.Get(&sent, `SELECT count(*) FROM outbox_events WHERE event_type = 'PEER_EXCHANGE' AND did = $1`, did))
	require.Equal(t, 1, sent)
	require.EqualValues(t, 1, peer.maxInFlight.Load(), "two ship attempts of one contract ran at the same time")
}

// CWE-7: a peer whose copy is closed answers gone. The sender records the
// revision as delivered, so it posts it once and leaves no retry entry.
func TestARevisionThePeerRefusesAsGoneIsShippedOnce(t *testing.T) {
	db := pgtest.Open(t)
	peer := startShipPeer(t, false)
	peer.refusal = dcstodcs.MakeGone(errors.New("the local copy is TERMINATED"))
	did := "ship-gone-" + strings.ToLower(rand.Text()[:8])
	s := shipFixture(t, db, peer, did)

	require.NoError(t, s.shipContractPDF(context.Background(), did))
	require.NoError(t, s.shipContractPDF(context.Background(), did))
	require.EqualValues(t, 1, peer.posts.Load())
	var retryEntries int
	require.NoError(t, db.Get(&retryEntries, `SELECT count(*) FROM sync_fails WHERE did = $1`, did))
	require.Zero(t, retryEntries)
}
