package dcstodcs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	goahttp "goa.design/goa/v3/http"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	dcstodcssvr "digital-contracting-service/gen/http/dcs_to_dcs/server"
	"digital-contracting-service/internal/base/pgtest"
	db2 "digital-contracting-service/internal/dcstodcs/db"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
)

const (
	conflictContractDID = "did:web:dcs-a.example:contract:conflict"
	conflictPeerDID     = "did:web:dcs-b.example"
)

// refusingPeer is a peer whose post_pdf answers with refusal.
type refusingPeer struct {
	dcstodcs.Service
	dcstodcs.Auther
	refusal error
}

func (p refusingPeer) PostPdf(context.Context, *dcstodcs.DCSToDCSContractPdfRequest) (*dcstodcs.DCSToDCSContractPdfResponse, error) {
	if p.refusal == nil {
		return &dcstodcs.DCSToDCSContractPdfResponse{FromPeerDid: conflictPeerDID}, nil
	}
	return nil, p.refusal
}

// peerClient serves refusal through the generated post_pdf HTTP server and
// returns the client the synchronizer ships with.
func peerClient(t *testing.T, refusal error) *dcstodcs.Client {
	t.Helper()
	mux := goahttp.NewMuxer()
	dcstodcssvr.Mount(mux, dcstodcssvr.New(dcstodcs.NewEndpoints(refusingPeer{refusal: refusal}), mux,
		goahttp.RequestDecoder, goahttp.ResponseEncoder, nil, goahttp.NewErrorResponse))
	peer := httptest.NewServer(http.StripPrefix("/api", mux))
	t.Cleanup(peer.Close)
	t.Setenv("DCS_API_PATH", "/api")
	return NewDCSToDCSHttpClient(strings.TrimPrefix(peer.URL, "http://"), "")
}

// shipOnce runs one ship attempt to the peer and settles its retry entry the
// way shipContractPDF does.
func shipOnce(t *testing.T, s *DCSToDCSSynchronizer, client *dcstodcs.Client) error {
	t.Helper()
	ctx := context.Background()
	shipErr := s.deliverPdf(ctx, client, db2.PDFDelivery{DID: conflictContractDID, PeerDID: conflictPeerDID, ContractState: "SIGNED", PDFCID: "cid-a"}, &dcstodcs.DCSToDCSContractPdfRequest{
		FromPeerDid: "did:web:dcs-a.example",
		ContractIri: conflictContractDID,
		Pdf:         []byte("%PDF-1.7 signed by A"),
		SecretValue: "challenge",
		SecretHash:  []byte("challenge signature"),
	})
	return s.recordShipOutcome(ctx, conflictContractDID, shipErr, nil)
}

func retryEntries(t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.Get(&count, `SELECT count(*) FROM sync_fails WHERE did = $1`, conflictContractDID))
	return count
}

func sentConflicts(t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.Get(&count, `
		SELECT count(*) FROM outbox_events
		WHERE event_type = 'PAC_SIGNING_CONFLICT' AND did = $1
		  AND event_data ->> 'peer_did' = $2 AND event_data ->> 'direction' = 'SENT'`,
		conflictContractDID, conflictPeerDID))
	return count
}

// The peer already holds its own signature over the same revision and refuses
// this instance's signed copy. A retry gets the same answer, so the refusal
// leaves no retry entry behind (also none an earlier failure left) and is
// recorded once as a signing conflict.
func TestASignedCopyThePeerRefusesAsAConflictIsRecordedAndNotRetried(t *testing.T) {
	db := pgtest.Open(t)
	s := &DCSToDCSSynchronizer{DB: db, SRepo: &syncpg.PostgresSyncRepository{}}
	require.Error(t, s.recordShipOutcome(context.Background(), conflictContractDID, errors.New("peer unreachable"), nil))
	require.Equal(t, 1, retryEntries(t, db))

	client := peerClient(t, dcstodcs.MakeConflict(errors.New("post_pdf rejected: signing conflict")))
	require.NoError(t, shipOnce(t, s, client))
	require.NoError(t, shipOnce(t, s, client))

	require.Zero(t, retryEntries(t, db), "a refused copy stays in the retry queue")
	require.Equal(t, 1, sentConflicts(t, db))
	require.Zero(t, deliveries(t, db), "a refused revision is offered again once the peer resolved the conflict")

	require.NoError(t, shipOnce(t, s, peerClient(t, nil)))
	require.Equal(t, 1, deliveries(t, db), "an accepted revision is not posted again")
}

func deliveries(t *testing.T, db *sqlx.DB) int {
	t.Helper()
	var count int
	require.NoError(t, db.Get(&count, `SELECT count(*) FROM contract_pdf_deliveries WHERE did = $1`, conflictContractDID))
	return count
}

// Any other refusal can change on a retry and stays in the retry queue.
func TestAShipThePeerRefusesForAnotherReasonIsRetried(t *testing.T) {
	db := pgtest.Open(t)
	s := &DCSToDCSSynchronizer{DB: db, SRepo: &syncpg.PostgresSyncRepository{}}

	client := peerClient(t, dcstodcs.MakeBadRequest(errors.New("post_pdf rejected: trust gate")))
	require.ErrorContains(t, shipOnce(t, s, client), "post_pdf rejected: trust gate")

	require.Equal(t, 1, retryEntries(t, db))
	require.Zero(t, sentConflicts(t, db))
}
