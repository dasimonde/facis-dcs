package command

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
)

// unsignedBase is the PDF both instances hold before either one signs.
var unsignedBase = []byte("%PDF-1.7\n1 0 obj << /Type /Catalog >> endobj\n%%EOF\n")

// withSignature appends a PAdES signature revision of signer to pdf.
func withSignature(pdf []byte, signer string) []byte {
	revision := fmt.Sprintf("9 0 obj << /Type /Sig /ByteRange [0 10 20 30] /Name (%s) >> endobj\n%%%%EOF\n", signer)
	return append(bytes.Clone(pdf), revision...)
}

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

// signingConflictReceiver is instance B: it received the unsigned contract
// from A and holds localPDF as its current copy.
func signingConflictReceiver(t *testing.T, localPDF []byte) (*PeerPdfReceiver, *sqlx.DB) {
	t.Helper()
	database := pgtest.Open(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	receiver := &PeerPdfReceiver{
		DB:     database,
		CRepo:  &cwepg.PostgresContractRepo{},
		RTRepo: &cwepg.PostgresReviewTaskRepo{},
		ATRepo: &cwepg.PostgresApprovalTaskRepo{},
		NTRepo: &cwepg.PostgresNegotiationTaskRepo{},
		Artifacts: artifactstore.New(&memoryObjects{blobs: map[string][]byte{}}, &artifactstore.PostgresCEKRepo{DB: database},
			ecdhAgreement{key}, localPeerDID, localPeerDID+"#agreement", &key.PublicKey),
	}
	ctx := context.Background()
	require.NoError(t, receiver.Handle(ctx, shipOf(unsignedBase)))
	if !bytes.Equal(localPDF, unsignedBase) {
		storeLocalPDF(t, receiver, localPDF)
	}
	return receiver, database
}

// storeLocalPDF stores pdf as the local copy, as the local signing path does.
func storeLocalPDF(t *testing.T, receiver *PeerPdfReceiver, pdf []byte) {
	t.Helper()
	ctx := context.Background()
	cid, err := receiver.Artifacts.Put(ctx, artifactstore.ContractScope(taskContractDID), pdf)
	require.NoError(t, err)
	tx, err := receiver.DB.BeginTxx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, receiver.CRepo.UpdatePDFState(ctx, tx, taskContractDID, db.ContractPDFState{IPFSCID: cid, C2PAState: "active"}))
	require.NoError(t, tx.Commit())
}

func shipOf(pdf []byte) PeerPdfReceiveCmd {
	cmd := peerShipCmd()
	cmd.Pdf = pdf
	return cmd
}

func localPDF(t *testing.T, receiver *PeerPdfReceiver) []byte {
	t.Helper()
	tx, err := receiver.DB.BeginTxx(context.Background(), nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	pdf, err := receiver.currentPDF(context.Background(), tx, taskContractDID)
	require.NoError(t, err)
	return pdf
}

func signingConflictEvents(t *testing.T, database *sqlx.DB) []string {
	t.Helper()
	var directions []string
	require.NoError(t, database.Select(&directions, `
		SELECT event_data ->> 'direction' FROM outbox_events
		WHERE event_type = 'PAC_SIGNING_CONFLICT' AND did = $1 AND event_data ->> 'peer_did' = $2`,
		taskContractDID, originPeerDID))
	return directions
}

// B signed and submitted first. A's copy, signed over the same unsigned
// revision, arrives afterwards: B refuses it, keeps its own signed copy and
// records the conflict once, however often A ships the copy.
func TestAPeerCopyThatDoesNotExtendTheLocalSignatureIsRefused(t *testing.T) {
	signedByB := withSignature(unsignedBase, "B")
	receiver, database := signingConflictReceiver(t, signedByB)
	ctx := context.Background()

	signedByA := withSignature(unsignedBase, "A")
	require.ErrorIs(t, receiver.Handle(ctx, shipOf(signedByA)), ErrSigningConflict)
	require.ErrorIs(t, receiver.Handle(ctx, shipOf(signedByA)), ErrSigningConflict)

	require.Equal(t, signedByB, localPDF(t, receiver), "the refused copy replaced the local signed copy")
	require.Equal(t, []string{"RECEIVED"}, signingConflictEvents(t, database))
}

// The sequential order: A's countersigned copy builds on B's signature.
func TestAPeerCopyThatExtendsTheLocalSignatureIsStored(t *testing.T) {
	signedByB := withSignature(unsignedBase, "B")
	receiver, database := signingConflictReceiver(t, signedByB)

	signedByBoth := withSignature(signedByB, "A")
	require.NoError(t, receiver.Handle(context.Background(), shipOf(signedByBoth)))

	require.Equal(t, signedByBoth, localPDF(t, receiver))
	require.Empty(t, signingConflictEvents(t, database))
}

// A copy without a local signature takes the peer's signed copy, as before.
func TestASignedPeerCopyReplacesAnUnsignedLocalCopy(t *testing.T) {
	receiver, database := signingConflictReceiver(t, unsignedBase)

	signedByA := withSignature(unsignedBase, "A")
	require.NoError(t, receiver.Handle(context.Background(), shipOf(signedByA)))

	require.Equal(t, signedByA, localPDF(t, receiver))
	require.Empty(t, signingConflictEvents(t, database))
}

// A late repeat of a revision the local copy already builds on changes nothing.
func TestAPeerCopyTheLocalCopyAlreadyContainsLeavesItUnchanged(t *testing.T) {
	signedByA := withSignature(unsignedBase, "A")
	signedByBoth := withSignature(signedByA, "B")
	receiver, database := signingConflictReceiver(t, signedByBoth)

	require.NoError(t, receiver.Handle(context.Background(), shipOf(signedByA)))

	require.Equal(t, signedByBoth, localPDF(t, receiver))
	require.Empty(t, signingConflictEvents(t, database))
}

// A revocation is adopted whatever revision it carries; the local signed copy
// stays.
func TestARevocationIsAdoptedWithoutReplacingTheLocalSignedCopy(t *testing.T) {
	signedByB := withSignature(unsignedBase, "B")
	receiver, database := signingConflictReceiver(t, signedByB)

	revocation := shipOf(withSignature(unsignedBase, "A"))
	revocation.AdoptRevoked = true
	require.NoError(t, receiver.Handle(context.Background(), revocation))

	require.Equal(t, signedByB, localPDF(t, receiver))
	var state string
	require.NoError(t, database.Get(&state, `SELECT state FROM contracts WHERE did = $1`, taskContractDID))
	require.Equal(t, contractstate.Revoked.String(), state)
	require.Empty(t, signingConflictEvents(t, database))
}

// The stored artifact answers whether it is signed: the flag is set when a
// peer's signed copy is stored, and a later update that records another
// asserted lifecycle state does not clear it, since an incremental update
// keeps every signature.
func TestAStoredSignedPeerCopyIsRecordedAsSigned(t *testing.T) {
	receiver, database := signingConflictReceiver(t, unsignedBase)
	ctx := context.Background()
	signed := func() bool {
		var flag bool
		require.NoError(t, database.Get(&flag, `SELECT pdf_signed FROM contracts WHERE did = $1`, taskContractDID))
		return flag
	}
	require.False(t, signed(), "an unsigned copy is recorded as signed")

	require.NoError(t, receiver.Handle(ctx, shipOf(withSignature(unsignedBase, "A"))))
	require.True(t, signed(), "the peer's signed copy is not recorded as signed")

	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, receiver.CRepo.UpdatePDFState(ctx, tx, taskContractDID, db.ContractPDFState{IPFSCID: "cid", C2PAState: "terminated"}))
	require.NoError(t, tx.Commit())
	require.True(t, signed(), "a lifecycle update cleared the signed flag")
}
