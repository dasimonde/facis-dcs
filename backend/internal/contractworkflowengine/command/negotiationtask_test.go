package command

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

const (
	taskContractDID = "did:web:facis.example:contract:round"
	originPeerDID   = "did:web:origin.example"
	localPeerHost   = "local.example"
	localPeerDID    = "did:web:" + localPeerHost
)

// -----------------------------------------------------------------------------
// Fakes
// -----------------------------------------------------------------------------

// negotiationTaskRepoFake models the round-keyed table: rows are addressed by
// (did, negotiator, contract_version) and Create absorbs a repeat mint exactly
// as the unique index does.
type negotiationTaskRepoFake struct {
	rows []db.NegotiationTaskData
}

func (r *negotiationTaskRepoFake) Create(_ context.Context, _ *sqlx.Tx, data db.NegotiationTaskData) (*time.Time, error) {
	for i := range r.rows {
		if r.rows[i].DID == data.DID && r.rows[i].Negotiator == data.Negotiator && r.rows[i].ContractVersion == data.ContractVersion {
			return &r.rows[i].CreatedAt, nil
		}
	}
	data.CreatedAt = time.Now().UTC()
	r.rows = append(r.rows, data)
	return &data.CreatedAt, nil
}

func (r *negotiationTaskRepoFake) IsValidNegotiator(_ context.Context, _ *sqlx.Tx, did string, negotiator string, contractVersion int) (bool, error) {
	for _, row := range r.rows {
		if row.DID == did && row.Negotiator == negotiator && row.ContractVersion == contractVersion {
			return true, nil
		}
	}
	return false, nil
}

func (r *negotiationTaskRepoFake) ReopenTasks(_ context.Context, _ *sqlx.Tx, did string, contractVersion int) error {
	for i := range r.rows {
		if r.rows[i].DID == did && r.rows[i].ContractVersion == contractVersion {
			r.rows[i].State = negotiationtaskstate.Open.String()
		}
	}
	return nil
}

func (r *negotiationTaskRepoFake) RollForward(_ context.Context, _ *sqlx.Tx, did string, fromVersion int, toVersion int) error {
	for i := range r.rows {
		if r.rows[i].DID == did && r.rows[i].ContractVersion == fromVersion {
			r.rows[i].ContractVersion = toVersion
			r.rows[i].State = negotiationtaskstate.Open.String()
		}
	}
	return nil
}

func (r *negotiationTaskRepoFake) ReadAllByNegotiator(_ context.Context, _ *sqlx.Tx, negotiator string) ([]db.NegotiationTaskData, error) {
	var out []db.NegotiationTaskData
	for _, row := range r.rows {
		if row.Negotiator == negotiator {
			out = append(out, row)
		}
	}
	return out, nil
}

func (r *negotiationTaskRepoFake) ReadNegotiatorsForDID(_ context.Context, _ *sqlx.Tx, did string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, row := range r.rows {
		if row.DID == did && !seen[row.Negotiator] {
			seen[row.Negotiator] = true
			out = append(out, row.Negotiator)
		}
	}
	return out, nil
}

func (r *negotiationTaskRepoFake) UpdateState(_ context.Context, _ *sqlx.Tx, did string, negotiator string, contractVersion int, state string) error {
	for i := range r.rows {
		if r.rows[i].DID == did && r.rows[i].Negotiator == negotiator && r.rows[i].ContractVersion == contractVersion {
			r.rows[i].State = state
			return nil
		}
	}
	return io.EOF // stands in for the repository's "no task for this round"
}

func (r *negotiationTaskRepoFake) AnyTasksInState(_ context.Context, _ *sqlx.Tx, did string, contractVersion int, states ...string) (bool, error) {
	for _, row := range r.rows {
		if row.DID != did || row.ContractVersion != contractVersion {
			continue
		}
		for _, state := range states {
			if row.State == state {
				return true, nil
			}
		}
	}
	return false, nil
}

type taskContractRepoFake struct {
	db.ContractRepo
	process        *db.ContractProcessData
	stored         *db.Contract
	pdfState       db.ContractPDFState
	states         []string
	updates        []db.ContractUpdateData
	historyEntries int
	// writes logs the version-bearing writes in order: a snapshot only
	// preserves the superseded document if it precedes the write replacing it.
	writes []string
}

// ReadProcessDataByDID hands out a copy, as the repository does: a handler
// holds the state it read for the length of its transaction, so a write it
// makes later must not retroactively change what it read.
func (r *taskContractRepoFake) ReadProcessDataByDID(context.Context, *sqlx.Tx, string) (*db.ContractProcessData, error) {
	snapshot := *r.process
	return &snapshot, nil
}

func (r *taskContractRepoFake) ReadDataByDID(context.Context, *sqlx.Tx, string) (*db.Contract, error) {
	return r.stored, nil
}

// Update stands in for the row: a later read in the same sequence sees what an
// earlier write left, so a propose-then-submit test runs against the document
// and the version the handlers actually produced.
func (r *taskContractRepoFake) Update(_ context.Context, _ *sqlx.Tx, data db.ContractUpdateData) error {
	r.updates = append(r.updates, data)
	if data.ContractData != nil {
		r.stored.ContractData = data.ContractData
	}
	if data.ContractVersion != 0 {
		r.process.ContractVersion = data.ContractVersion
		r.writes = append(r.writes, "update")
	}
	return nil
}

func (r *taskContractRepoFake) ReadPDFState(context.Context, *sqlx.Tx, string) (*db.ContractPDFState, error) {
	return &r.pdfState, nil
}

// versionBumps returns the contract versions the handlers wrote, in order. A
// reseed writes contract data without a version and is not a bump.
func (r *taskContractRepoFake) versionBumps() []int {
	var out []int
	for _, update := range r.updates {
		if update.ContractVersion != 0 {
			out = append(out, update.ContractVersion)
		}
	}
	return out
}

func (r *taskContractRepoFake) CreateHistoryEntryForDID(context.Context, *sqlx.Tx, string) error {
	r.historyEntries++
	r.writes = append(r.writes, "history")
	return nil
}

func (r *taskContractRepoFake) UpdateState(_ context.Context, _ *sqlx.Tx, _ string, state string) error {
	r.states = append(r.states, state)
	r.process.State = state
	return nil
}

// negotiationRepoFake answers the two round predicates submit consults. A round
// with no proposed change requests is the plain accept-and-settle case.
type negotiationRepoFake struct {
	db.NegotiationRepo
	openDecisions bool
	negotiations  bool
	// accepted is what a merge folds in, keyed by the round it was accepted on.
	accepted map[int][]db.NegotiationChangeData
	// proposed are the rows negotiate recorded, each on the round it was
	// proposed against.
	proposed []proposedChange
	// superseded are the annotations the merge left on accepted requests whose
	// content it discarded.
	superseded []db.NegotiationSupersession
}

func (r *negotiationRepoFake) MarkSuperseded(_ context.Context, _ *sqlx.Tx, supersessions []db.NegotiationSupersession) error {
	r.superseded = append(r.superseded, supersessions...)
	return nil
}

// proposedChange is one contract_negotiations row: the change request and the
// id an accept addresses it by.
type proposedChange struct {
	id   string
	data db.NegotiationCreateData
}

func (r *negotiationRepoFake) Create(_ context.Context, _ *sqlx.Tx, data db.NegotiationCreateData, _ []string) (*time.Time, error) {
	r.proposed = append(r.proposed, proposedChange{id: fmt.Sprintf("negotiation-%d", len(r.proposed)+1), data: data})
	now := time.Now().UTC()
	return &now, nil
}

func (r *negotiationRepoFake) Accept(_ context.Context, _ *sqlx.Tx, id string, _ string) error {
	for _, row := range r.proposed {
		if row.id != id {
			continue
		}
		if r.accepted == nil {
			r.accepted = map[int][]db.NegotiationChangeData{}
		}
		r.accepted[row.data.ContractVersion] = append(r.accepted[row.data.ContractVersion],
			db.NegotiationChangeData{ID: row.id, ChangeRequest: row.data.ChangeRequest})
		return nil
	}
	return db.ErrNoMatchingDecision
}

func (r *negotiationRepoFake) ReadCreatedByByNegotiationID(_ context.Context, _ *sqlx.Tx, id string) (string, error) {
	for _, row := range r.proposed {
		if row.id == id {
			return row.data.CreatedBy, nil
		}
	}
	return "", db.ErrNoMatchingDecision
}

func (r *negotiationRepoFake) DeleteDraft(context.Context, *sqlx.Tx, string, string) error {
	return nil
}

func (r *negotiationRepoFake) HasOpenNegotiationDecisions(context.Context, *sqlx.Tx, string, int, string, string) (bool, error) {
	return r.openDecisions, nil
}

func (r *negotiationRepoFake) HasNegotiationForContractVersion(_ context.Context, _ *sqlx.Tx, _ string, contractVersion int) (bool, error) {
	for _, row := range r.proposed {
		if row.data.ContractVersion == contractVersion {
			return true, nil
		}
	}
	return r.negotiations, nil
}

func (r *negotiationRepoFake) ReadAllAcceptedByContractDIDAndVersion(_ context.Context, _ *sqlx.Tx, _ string, contractVersion int) ([]db.NegotiationChangeData, error) {
	return r.accepted[contractVersion], nil
}

// outboxOnlyDriver hands out connections that open a transaction and swallow
// the outbox INSERT. Every read and write the handlers make goes through the
// repository fakes above; only event.Create reaches the database.
type outboxOnlyDriver struct{}

func (outboxOnlyDriver) Open(string) (driver.Conn, error) { return outboxOnlyConn{}, nil }

type outboxOnlyConn struct{}

func (outboxOnlyConn) Prepare(string) (driver.Stmt, error) { return outboxOnlyStmt{}, nil }
func (outboxOnlyConn) Close() error                        { return nil }
func (outboxOnlyConn) Begin() (driver.Tx, error)           { return outboxOnlyTx{}, nil }

type outboxOnlyStmt struct{}

func (outboxOnlyStmt) Close() error  { return nil }
func (outboxOnlyStmt) NumInput() int { return -1 }
func (outboxOnlyStmt) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}
func (outboxOnlyStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, io.EOF
}

type outboxOnlyTx struct{}

func (outboxOnlyTx) Commit() error   { return nil }
func (outboxOnlyTx) Rollback() error { return nil }

func init() { sql.Register("negotiation-task-outbox-only", outboxOnlyDriver{}) }

func taskTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	raw, err := sql.Open("negotiation-task-outbox-only", "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	return sqlx.NewDb(raw, "postgres")
}

// taskTestDIDDocument builds the local instance's DID document through the same
// NewDIDDocument path production uses.
func taskTestDIDDocument(t *testing.T) *identity.DIDDocument {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	didJSON := map[string]any{
		"id":             localPeerDID,
		"authentication": []any{"#key-1"},
		"verificationMethod": []map[string]any{
			{
				"id": localPeerDID + "#key-1",
				"publicKeyJwk": map[string]any{
					"kty": "EC",
					"crv": "P-256",
					"x":   base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
					"y":   base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
				},
			},
		},
	}
	raw, err := json.Marshal(didJSON)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "did.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	doc, err := identity.NewDIDDocument(path, key)
	require.NoError(t, err)
	return doc
}

// inboundOffer is the counterparty's copy of a received offer: the origin is
// the peer, and the local instance holds no negotiation task for it.
func inboundOffer(version int) *db.ContractProcessData {
	return &db.ContractProcessData{
		DID:             taskContractDID,
		Origin:          originPeerDID,
		ContractVersion: version,
		State:           contractstate.Offered.String(),
		CreatedBy:       originPeerDID,
	}
}

func taskStoredContract() *db.Contract {
	data := datatype.JSON(`{"@id":"` + taskContractDID + `","@type":"dcs:Contract"}`)
	return &db.Contract{
		DID:          taskContractDID,
		ContractData: &data,
		Responsible:  &db.Responsible{Creator: originPeerDID, Counterparty: localPeerDID},
	}
}

func offerAcceptor(t *testing.T, process *db.ContractProcessData) (*OfferAcceptor, *taskContractRepoFake, *negotiationTaskRepoFake) {
	t.Helper()
	contracts := &taskContractRepoFake{process: process, stored: taskStoredContract()}
	tasks := &negotiationTaskRepoFake{}
	return &OfferAcceptor{
		DB:          taskTestDB(t),
		CRepo:       contracts,
		NTRepo:      tasks,
		DIDDocument: *taskTestDIDDocument(t),
	}, contracts, tasks
}

func acceptOfferCmd() AcceptOfferCmd {
	return AcceptOfferCmd{
		DID:        taskContractDID,
		AcceptedBy: "Counterparty Org",
		UpdatedAt:  time.Now().UTC().Add(time.Hour),
		HolderDID:  "did:key:user",
		UserRoles:  userrole.UserRoles{userrole.ContractNegotiator},
		CauserDID:  localPeerDID,
	}
}

// -----------------------------------------------------------------------------
// Accepting an offer is what mints the task
// -----------------------------------------------------------------------------

// The defect this whole change answers: an offer the counterparty accepted
// produced no negotiation task, so its Negotiations tab stayed empty and
// submit's "no open tasks" gate answered for a round nobody had entered.
func TestAcceptingAnInboundOfferMintsThisInstancesTaskAndOpensNegotiation(t *testing.T) {
	handler, contracts, tasks := offerAcceptor(t, inboundOffer(1))

	require.NoError(t, handler.Handle(context.Background(), acceptOfferCmd()))

	require.Len(t, tasks.rows, 1)
	require.Equal(t, localPeerDID, tasks.rows[0].Negotiator, "a task never names the peer (ADR-13)")
	require.Equal(t, 1, tasks.rows[0].ContractVersion)
	require.Equal(t, negotiationtaskstate.Open.String(), tasks.rows[0].State,
		"accepting the OFFER enters the round; accepting the ROUND is Submit")
	require.Equal(t, []string{contractstate.Negotiation.String()}, contracts.states)
}

// A task belongs to the round on the table, not to the contract's first round:
// each accepted redline bumps the version and starts a new one.
func TestAcceptingAnOfferMintsForTheContractsCurrentRound(t *testing.T) {
	handler, _, tasks := offerAcceptor(t, inboundOffer(4))

	require.NoError(t, handler.Handle(context.Background(), acceptOfferCmd()))

	require.Len(t, tasks.rows, 1)
	require.Equal(t, 4, tasks.rows[0].ContractVersion)
}

// Double-clicking accept, or accepting a contract already in NEGOTIATION, must
// not stack up tasks — the second accept is a legal self-loop that writes
// nothing.
func TestAcceptingAnOfferTwiceMintsOneTask(t *testing.T) {
	handler, contracts, tasks := offerAcceptor(t, inboundOffer(1))

	require.NoError(t, handler.Handle(context.Background(), acceptOfferCmd()))
	contracts.process.State = contractstate.Negotiation.String()
	require.NoError(t, handler.Handle(context.Background(), acceptOfferCmd()))

	require.Len(t, tasks.rows, 1)
}

// The origin has held a task since it authored the contract; "accept the offer"
// is the Responder's act and the origin accepting its own offer is not a party
// question the table can answer.
func TestTheOriginCannotAcceptItsOwnOffer(t *testing.T) {
	own := inboundOffer(1)
	own.Origin = localPeerDID
	handler, _, tasks := offerAcceptor(t, own)

	require.ErrorIs(t, handler.Handle(context.Background(), acceptOfferCmd()), ErrNotAParty)
	require.Empty(t, tasks.rows)
}

// -----------------------------------------------------------------------------
// The settlement gate
// -----------------------------------------------------------------------------

func submitter(t *testing.T, process *db.ContractProcessData, tasks *negotiationTaskRepoFake, negotiations *negotiationRepoFake) (*Submitter, *taskContractRepoFake) {
	t.Helper()
	contracts := &taskContractRepoFake{process: process, stored: taskStoredContract()}
	return &Submitter{
		DB:          taskTestDB(t),
		CRepo:       contracts,
		NRepo:       negotiations,
		NTRepo:      tasks,
		DIDDocument: *taskTestDIDDocument(t),
	}, contracts
}

func negotiationSubmitCmd() SubmitCmd {
	return SubmitCmd{
		DID:         taskContractDID,
		UpdatedAt:   time.Now().UTC().Add(time.Hour),
		SubmittedBy: "Counterparty Org",
		HolderDID:   "did:key:user",
		UserRoles:   userrole.UserRoles{userrole.ContractNegotiator},
		CauserDID:   localPeerDID,
	}
}

func inNegotiation(version int) *db.ContractProcessData {
	process := inboundOffer(version)
	process.State = contractstate.Negotiation.String()
	return process
}

// Symptom 3 of the defect: submit walked NEGOTIATION -> SUBMITTED on the first
// click because "no open tasks" was trivially true when no task existed. The
// refusal now names the actual condition.
func TestSubmitRefusesARoundThisInstanceNeverEntered(t *testing.T) {
	handler, contracts := submitter(t, inNegotiation(1), &negotiationTaskRepoFake{}, &negotiationRepoFake{})

	err := handler.Handle(context.Background(), negotiationSubmitCmd())

	require.ErrorIs(t, err, ErrNegotiationNotSettled)
	require.Empty(t, contracts.states, "a refused submit moves no state")
}

// A task for the CURRENT round is what counts: one left behind on a superseded
// version is not evidence that anyone engaged with this one.
func TestSubmitRefusesWhenTheOnlyTaskBelongsToAnEarlierRound(t *testing.T) {
	tasks := &negotiationTaskRepoFake{rows: []db.NegotiationTaskData{{
		DID:             taskContractDID,
		ContractVersion: 1,
		Negotiator:      localPeerDID,
		State:           negotiationtaskstate.Accepted.String(),
	}}}
	handler, contracts := submitter(t, inNegotiation(2), tasks, &negotiationRepoFake{})

	require.ErrorIs(t, handler.Handle(context.Background(), negotiationSubmitCmd()), ErrNegotiationNotSettled)
	require.Empty(t, contracts.states)
}

// Once the round has been entered and nothing is left to merge, submitting
// accepts this instance's task and settles the round.
func TestSubmitSettlesTheRoundOnceTheEnteredTaskIsAccepted(t *testing.T) {
	tasks := &negotiationTaskRepoFake{rows: []db.NegotiationTaskData{{
		DID:             taskContractDID,
		ContractVersion: 2,
		Negotiator:      localPeerDID,
		State:           negotiationtaskstate.Open.String(),
	}}}
	handler, contracts := submitter(t, inNegotiation(2), tasks, &negotiationRepoFake{})

	require.NoError(t, handler.Handle(context.Background(), negotiationSubmitCmd()))

	require.Equal(t, []string{contractstate.Submitted.String()}, contracts.states)
	require.Equal(t, negotiationtaskstate.Accepted.String(), tasks.rows[0].State)
}

// Accepting the caller's own task settles the round only when it is the last
// one open. create.go mints one task per entry in resp.Negotiators, so a round
// can carry more than one, and the submitter must not settle for the others.
func TestSubmitDoesNotSettleWhileAnotherNegotiatorsTaskIsOpen(t *testing.T) {
	const secondNegotiator = localPeerDID + ":negotiator:second"
	tasks := &negotiationTaskRepoFake{rows: []db.NegotiationTaskData{
		{
			DID:             taskContractDID,
			ContractVersion: 2,
			Negotiator:      localPeerDID,
			State:           negotiationtaskstate.Open.String(),
		},
		{
			DID:             taskContractDID,
			ContractVersion: 2,
			Negotiator:      secondNegotiator,
			State:           negotiationtaskstate.Open.String(),
		},
	}}
	handler, contracts := submitter(t, inNegotiation(2), tasks, &negotiationRepoFake{})

	require.NoError(t, handler.Handle(context.Background(), negotiationSubmitCmd()))

	require.Equal(t, negotiationtaskstate.Accepted.String(), tasks.rows[0].State)
	require.Equal(t, negotiationtaskstate.Open.String(), tasks.rows[1].State)
	require.Empty(t, contracts.states,
		"a round with a task still open is not settled, whoever holds that task")
}

// Merging accepted redlines bumps contract_version, which starts a new round.
// Tasks key on the round, so they must travel to the new version and reopen —
// otherwise the party that engaged with the merged round silently loses its
// standing and its next submit is refused as a round it never entered.
func TestMergingARedlineOpensANewRoundTheEngagedPartyCanStillSettle(t *testing.T) {
	renamed := datatype.JSON(`{"name":"Renegotiated Service Level"}`)
	negotiations := &negotiationRepoFake{
		negotiations: true,
		accepted: map[int][]db.NegotiationChangeData{
			2: {{ID: "change-1", ChangeRequest: &renamed}},
		},
	}
	tasks := &negotiationTaskRepoFake{rows: []db.NegotiationTaskData{{
		DID:             taskContractDID,
		ContractVersion: 2,
		Negotiator:      localPeerDID,
		State:           negotiationtaskstate.Open.String(),
	}}}
	handler, contracts := submitter(t, inNegotiation(2), tasks, negotiations)

	require.NoError(t, handler.Handle(context.Background(), negotiationSubmitCmd()))

	require.Len(t, contracts.updates, 1)
	require.Equal(t, 3, contracts.updates[0].ContractVersion)
	require.Equal(t, 1, contracts.historyEntries)
	require.Equal(t, 3, tasks.rows[0].ContractVersion, "the task belongs to the round the merge opened")
	require.Equal(t, negotiationtaskstate.Open.String(), tasks.rows[0].State,
		"the merged document is a new document to respond to")
	require.Empty(t, contracts.states, "a merge opens the next round, it does not settle the contract")

	// The round the merge opened is settleable by the party that engaged with the
	// previous one — which is the whole point of carrying the task forward.
	contracts.process.ContractVersion = 3
	negotiations.negotiations = false

	require.NoError(t, handler.Handle(context.Background(), negotiationSubmitCmd()))

	require.Equal(t, []string{contractstate.Submitted.String()}, contracts.states)
}

// -----------------------------------------------------------------------------
// Structural invariants
// -----------------------------------------------------------------------------

// The decision the whole design rests on: receiving an offer queues nothing. A
// task minted on arrival would make submit's gate true the moment an offer
// lands and would put work in a tab nobody chose to open.
func TestReceivingAPeerContractMintsNoNegotiationTask(t *testing.T) {
	require.NotContains(t, callsIn(t, "receivepdf.go", "Handle"), "mintNegotiationTask",
		"a negotiation task must be minted by an act of engagement, never by receipt")
}

// A version bump starts a new round, and a task keys on the round. Every
// handler that bumps contract_version must carry the tasks across, or the party
// that engaged with the last round silently loses its standing in the next one
// and can never submit again.
func TestEveryContractVersionBumpCarriesTheNegotiationTasksForward(t *testing.T) {
	for file, fn := range map[string]string{
		"submit.go":     "Handle",
		"receivepdf.go": "Handle",
		"negotiate.go":  "Handle",
	} {
		calls := callsIn(t, file, fn)
		require.Contains(t, calls, "RollForward",
			"%s bumps the contract version without carrying the negotiation tasks to the new round", file)
	}
}

// callsIn returns the names of every function and method called anywhere inside
// the named function of the named file in this package.
func callsIn(t *testing.T, file, fnName string) map[string]bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)

	calls := map[string]bool{}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != fnName || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch target := call.Fun.(type) {
			case *ast.Ident:
				calls[target.Name] = true
			case *ast.SelectorExpr:
				calls[target.Sel.Name] = true
			}
			return true
		})
		return calls
	}
	t.Fatalf("%s has no function %s", file, fnName)
	return nil
}
