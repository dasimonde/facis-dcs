package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/command"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	trustgate "digital-contracting-service/internal/dcstodcs"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
)

type capturingTerminationSender struct {
	sent []*dcstodcs.DCSToDCSContractTerminateRequest
	fail error
}

func (s *capturingTerminationSender) SendTermination(_ context.Context, _ string, req *dcstodcs.DCSToDCSContractTerminateRequest) error {
	s.sent = append(s.sent, req)
	return s.fail
}

// DCS-NFR-BR-06: a local termination queues a notice toward the counterparty
// instance that received the contract, in the same transaction, and the
// notifier delivers it until the peer confirms. The signed challenge binds the
// contract, the receiver and the reason.
func TestTerminationIsPropagatedToThePeerPostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := eraseTestDIDDocument(t, "local.example")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,responsible) VALUES ('c1',$1,'Org A','ACTIVE','tpl',1,
		jsonb_build_object('creator',$2::text,'counterparty','did:web:peer.example'))`, peer, peer)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_pdf_deliveries (did, peer_did, contract_state, pdf_cid) VALUES ('c1', 'did:web:peer.example', 'SIGNED', 'cid')`)
	require.NoError(t, err)

	handler := command.Terminator{DB: database, CRepo: &cwepg.PostgresContractRepo{}, SRepo: syncpg.PostgresSyncRepository{}, DIDDocument: *document}
	require.NoError(t, handler.Handle(context.Background(), command.TerminateCmd{
		DID: "c1", TerminatedBy: "Org A", Reason: "breach", UpdatedAt: time.Now().Add(time.Hour), CauserDID: peer,
		UserRoles: userrole.UserRoles{userrole.ContractManager},
	}))

	sender := &capturingTerminationSender{fail: errors.New("peer down")}
	notifier := trustgate.TerminationNotifier{DB: database, DIDDocument: *document, Sender: sender}
	notifier.DeliverPending(context.Background())
	require.Len(t, sender.sent, 1)
	require.Equal(t, "c1", sender.sent[0].ContractIri)
	require.Equal(t, "breach", *sender.sent[0].Reason)
	require.NoError(t, trustgate.VerifyPeerRequest(context.Background(), memoryNonces{}, document, nil, "did:web:peer.example", trustgate.PeerRequest{
		Op: trustgate.OpTerminate, FromDID: peer, ContractIRI: "c1", Body: []byte("breach"),
		SecretValue: sender.sent[0].SecretValue, SecretHash: sender.sent[0].SecretHash,
	}, time.Now()))

	var retries int
	require.NoError(t, database.Get(&retries, `SELECT retry_count FROM contract_peer_terminations WHERE did='c1' AND peer_did='did:web:peer.example' AND confirmed_at IS NULL`))
	require.Equal(t, 1, retries)

	_, err = database.Exec(`UPDATE contract_peer_terminations SET last_tried_at = now() - INTERVAL '1 hour'`)
	require.NoError(t, err)
	sender.fail = nil
	notifier.DeliverPending(context.Background())
	var confirmed int
	require.NoError(t, database.Get(&confirmed, `SELECT count(*) FROM contract_peer_terminations WHERE confirmed_at IS NOT NULL`))
	require.Equal(t, 1, confirmed)
}

// The receiving side terminates its copy only on notice from a party, does not
// notify back, and confirms a repeated notice.
func TestPeerTerminationNoticeTerminatesTheLocalCopyPostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	local, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,responsible) VALUES ('c1','did:web:peer.example','Org B','ACTIVE','tpl',1,
		jsonb_build_object('creator','did:web:peer.example','counterparty',$1::text))`, local)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_review_task (did,state,reviewer,created_by) VALUES ('c1','OPEN','r1','Org B')`)
	require.NoError(t, err)
	reason := "breach"
	notice := func(from string) *dcstodcs.DCSToDCSContractTerminateRequest {
		return &dcstodcs.DCSToDCSContractTerminateRequest{FromPeerDid: from, ContractIri: "c1", Reason: &reason}
	}
	terminate := func(from string) error {
		return terminateForPeer(context.Background(), database, &cwepg.PostgresContractRepo{}, syncpg.PostgresSyncRepository{}, *document, notice(from))
	}

	require.Error(t, terminate("did:web:stranger.example"))
	var state string
	require.NoError(t, database.Get(&state, `SELECT state FROM contracts WHERE did='c1'`))
	require.Equal(t, "ACTIVE", state)

	require.NoError(t, terminate("did:web:peer.example"))
	require.NoError(t, database.Get(&state, `SELECT state FROM contracts WHERE did='c1'`))
	require.Equal(t, "TERMINATED", state)
	var open, queued int
	require.NoError(t, database.Get(&open, `SELECT count(*) FROM contract_review_task WHERE state='OPEN'`))
	require.Zero(t, open)
	require.NoError(t, database.Get(&queued, `SELECT count(*) FROM contract_peer_terminations`))
	require.Zero(t, queued, "a received termination is not sent back")

	require.NoError(t, terminate("did:web:peer.example"))
}

// CWE-14: a notice for a copy that is already out of force (here EXPIRED) is
// answered as delivered and leaves the copy as it is, so the sender stops
// sending it.
func TestPeerTerminationNoticeForAClosedCopyIsConfirmedPostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	local, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,responsible) VALUES ('c1','did:web:peer.example','Org B','EXPIRED','tpl',1,
		jsonb_build_object('creator','did:web:peer.example','counterparty',$1::text))`, local)
	require.NoError(t, err)
	reason := "breach"

	require.NoError(t, terminateForPeer(context.Background(), database, &cwepg.PostgresContractRepo{}, syncpg.PostgresSyncRepository{}, *document,
		&dcstodcs.DCSToDCSContractTerminateRequest{FromPeerDid: "did:web:peer.example", ContractIri: "c1", Reason: &reason}))

	var state string
	require.NoError(t, database.Get(&state, `SELECT state FROM contracts WHERE did='c1'`))
	require.Equal(t, "EXPIRED", state)
}

// A notice whose signed challenge names another contract is refused.
func TestPeerTerminationNoticeMustNameTheContract(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, _, restore := newEraseTestService(t, peerDoc, []string{eraseTestPeer})
	defer restore()
	reason := "breach"
	value, hash := signedFor(t, peerDoc, trustgate.OpTerminate, eraseTestPeer, eraseTestLocal, "c2", []byte(reason))
	_, err := svc.Terminate(context.Background(), &dcstodcs.DCSToDCSContractTerminateRequest{
		FromPeerDid: eraseTestPeer, ContractIri: "c1", Reason: &reason, SecretValue: value, SecretHash: hash,
	})
	require.ErrorContains(t, err, "names contract c2, not c1")
}

// DCS-FR-CWE-23: a contract that never reached the counterparty (a DRAFT
// terminated before its offer) queues no termination notice toward it.
func TestTerminatingADraftQueuesNoPeerNoticePostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	local, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,responsible) VALUES ('c1',$1::text,'Org A','DRAFT','tpl',1,
		jsonb_build_object('creator',$1::text,'counterparty','did:web:peer.example'))`, local)
	require.NoError(t, err)

	handler := command.Terminator{DB: database, CRepo: &cwepg.PostgresContractRepo{}, SRepo: syncpg.PostgresSyncRepository{}, DIDDocument: *document}
	require.NoError(t, handler.Handle(context.Background(), command.TerminateCmd{
		DID: "c1", TerminatedBy: "Org A", Reason: "not needed", UpdatedAt: time.Now().Add(time.Hour), CauserDID: local,
		UserRoles: userrole.UserRoles{userrole.ContractManager},
	}))
	var queued int
	require.NoError(t, database.Get(&queued, `SELECT count(*) FROM contract_peer_terminations`))
	require.Zero(t, queued, "a notice was queued toward a peer that never received the contract")
}

// A peer that holds no copy shared with this instance refuses the notice for
// good: the notice is not sent again, and the contract's lifecycle shows the
// refusal. Delivered notices are audited (DCS-NFR-BR-08).
func TestNotifierStopsOnPermanentRefusalPostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	_, err := database.Exec(`INSERT INTO contract_peer_terminations (did, peer_did, reason) VALUES
		('c1', 'did:web:stranger.example', 'breach'), ('c1', 'did:web:peer.example', 'breach')`)
	require.NoError(t, err)

	sender := &refusingTerminationSender{refuse: "did:web:stranger.example"}
	notifier := trustgate.TerminationNotifier{DB: database, DIDDocument: *document, Sender: sender}
	notifier.DeliverPending(context.Background())
	_, err = database.Exec(`UPDATE contract_peer_terminations SET last_tried_at = now() - INTERVAL '1 hour'`)
	require.NoError(t, err)
	notifier.DeliverPending(context.Background())

	require.Equal(t, map[string]int{"did:web:stranger.example": 1, "did:web:peer.example": 1}, sender.attempts)
	var refused, confirmed int
	require.NoError(t, database.Get(&refused, `SELECT count(*) FROM contract_peer_terminations WHERE refused_at IS NOT NULL AND last_error LIKE '%no contract%'`))
	require.NoError(t, database.Get(&confirmed, `SELECT count(*) FROM contract_peer_terminations WHERE confirmed_at IS NOT NULL`))
	require.Equal(t, 1, refused)
	require.Equal(t, 1, confirmed)

	refusals := archiveEvents(t, database, "PEER_EXCHANGE_REFUSED")
	require.Len(t, refusals, 1)
	require.Equal(t, "did:web:stranger.example", refusals[0]["peer_did"])
	exchanges := archiveEvents(t, database, "PEER_EXCHANGE")
	require.Len(t, exchanges, 1)
	require.Equal(t, "terminate", exchanges[0]["exchange"])
	require.Equal(t, "SENT", exchanges[0]["direction"])
}

type refusingTerminationSender struct {
	refuse   string
	attempts map[string]int
}

func (s *refusingTerminationSender) SendTermination(_ context.Context, peerDID string, _ *dcstodcs.DCSToDCSContractTerminateRequest) error {
	if s.attempts == nil {
		s.attempts = map[string]int{}
	}
	s.attempts[peerDID]++
	if peerDID == s.refuse {
		return dcstodcs.MakeNotFound(errors.New("termination rejected: this instance holds no contract c1 shared with the peer"))
	}
	return nil
}
