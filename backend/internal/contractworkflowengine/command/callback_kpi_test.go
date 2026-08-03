package command

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

const (
	kpiContractDID = "did:web:example:contract:kpi"
	availabilityID = "urn:uuid:policy-availability"
	compensateID   = "urn:uuid:policy-compensate"
)

// kpiContract carries one obligation and one permission whose duty is nested,
// which is how a compensation term reaches the target: inside the rule it hangs
// off, not as a bucket entry of its own.
func kpiContract() string {
	return `{
	  "@id": "` + kpiContractDID + `",
	  "@type": "dcs:Contract",
	  "dcs:policies": {
	    "@id": "urn:uuid:policy-set",
	    "@type": "odrl:Agreement",
	    "odrl:obligation": [{
	      "@id": "` + availabilityID + `",
	      "@type": "odrl:Duty",
	      "odrl:action": {"@id": "dcs:provideCompliantValue"}
	    }],
	    "odrl:permission": [{
	      "@id": "urn:uuid:policy-use",
	      "@type": "odrl:Permission",
	      "odrl:action": {"@id": "odrl:use"},
	      "odrl:duty": [{"@id": "` + compensateID + `", "@type": "odrl:Duty"}]
	    }]
	  }
	}`
}

type kpiContractRepoFake struct {
	db.ContractRepo
	contract *db.Contract
	reads    int
}

func (r *kpiContractRepoFake) ReadDataByDID(context.Context, *sqlx.Tx, string) (*db.Contract, error) {
	r.reads++
	return r.contract, nil
}

type kpiDeploymentRepoFake struct {
	deployment *db.ContractDeployment
	recorded   []db.ContractKPI
}

func (r *kpiDeploymentRepoFake) CreateDeployment(context.Context, *sqlx.Tx, db.ContractDeployment) error {
	panic("not used by the callback path")
}

func (r *kpiDeploymentRepoFake) FindDeploymentByCorrelationID(_ context.Context, _ *sqlx.Tx, _ string) (*db.ContractDeployment, error) {
	return r.deployment, nil
}

func (r *kpiDeploymentRepoFake) AcknowledgeDeployment(context.Context, *sqlx.Tx, string, string, string, time.Time) error {
	panic("not used by a KPI report")
}

func (r *kpiDeploymentRepoFake) MarkDispatchFailed(context.Context, *sqlx.Tx, string, string) error {
	panic("not used by the callback path")
}

func (r *kpiDeploymentRepoFake) CreateKPI(_ context.Context, _ *sqlx.Tx, data db.ContractKPI) error {
	r.recorded = append(r.recorded, data)
	return nil
}

func (r *kpiDeploymentRepoFake) ReadKPIsByDID(context.Context, *sqlx.Tx, string) ([]db.ContractKPI, error) {
	panic("not used by the callback path")
}

// noQueryDriver hands out connections that can open and close a transaction and
// nothing else: the repositories below are fakes, so no statement ever reaches
// the database, but Handle still needs a real *sqlx.DB to start the transaction
// it passes down.
type noQueryDriver struct{}

func (noQueryDriver) Open(string) (driver.Conn, error) { return noQueryConn{}, nil }

type noQueryConn struct{}

func (noQueryConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("no statement may be prepared here")
}
func (noQueryConn) Close() error              { return nil }
func (noQueryConn) Begin() (driver.Tx, error) { return noQueryTx{}, nil }

type noQueryTx struct{}

func (noQueryTx) Commit() error   { return nil }
func (noQueryTx) Rollback() error { return nil }

func init() { sql.Register("callback-no-query", noQueryDriver{}) }

func kpiHandler(t *testing.T, contractData *string) (*DeploymentCallbackHandler, *kpiDeploymentRepoFake, *kpiContractRepoFake) {
	t.Helper()

	raw, err := sql.Open("callback-no-query", "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })

	contract := &db.Contract{DID: kpiContractDID}
	if contractData != nil {
		document := datatype.JSON(*contractData)
		contract.ContractData = &document
	}
	contractRepo := &kpiContractRepoFake{contract: contract}
	deploymentRepo := &kpiDeploymentRepoFake{deployment: &db.ContractDeployment{
		DID:           kpiContractDID,
		CorrelationID: "corr-1",
		ContentHash:   "sha256:deadbeef",
		TargetID:      ptr("t-1"),
	}}

	return &DeploymentCallbackHandler{
		DB:             sqlx.NewDb(raw, "postgres"),
		CRepo:          contractRepo,
		DeploymentRepo: deploymentRepo,
		TargetRepo:     &stubTargetRepo{target: &db.ContractTarget{ID: "t-1", OAuthClientID: ptr("dcs-target-t-1")}},
	}, deploymentRepo, contractRepo
}

func kpiReport(verdict, rule string) DeploymentCallbackCmd {
	return DeploymentCallbackCmd{
		CallerClientID: "dcs-target-t-1",
		DID:            kpiContractDID,
		CorrelationID:  "corr-1",
		KPIMetric:      "urn:uuid:field-availability",
		KPIValue:       "80",
		KPIVerdict:     verdict,
		KPIRule:        rule,
	}
}

// The headline of ADR-33: the target concluded, the DCS records what it
// concluded and which rule it concluded about.
func TestKPIReportRecordsAViolatedVerdictAgainstTheRuleItNames(t *testing.T) {
	handler, deployments, _ := kpiHandler(t, ptr(kpiContract()))

	require.NoError(t, handler.Handle(context.Background(), kpiReport(db.KPIVerdictViolated, availabilityID)))

	require.Len(t, deployments.recorded, 1)
	recorded := deployments.recorded[0]
	require.Equal(t, db.KPIVerdictViolated, recorded.Verdict)
	require.NotNil(t, recorded.RuleID, "a violation the DCS cannot trace to a rule is not evidence")
	require.Equal(t, availabilityID, *recorded.RuleID)
	require.Equal(t, "urn:uuid:field-availability", recorded.Metric)
	require.Equal(t, "80", recorded.Value)
}

func TestKPIReportRecordsASatisfiedVerdictAsSatisfied(t *testing.T) {
	handler, deployments, _ := kpiHandler(t, ptr(kpiContract()))

	require.NoError(t, handler.Handle(context.Background(), kpiReport(db.KPIVerdictSatisfied, availabilityID)))

	require.Len(t, deployments.recorded, 1)
	require.Equal(t, db.KPIVerdictSatisfied, deployments.recorded[0].Verdict)
	require.Equal(t, availabilityID, *deployments.recorded[0].RuleID)
}

// A duty nested under a permission is a term of the contract that reached the
// target inside the rule it hangs off, so a verdict may name it.
func TestKPIReportMayNameANestedDuty(t *testing.T) {
	handler, deployments, _ := kpiHandler(t, ptr(kpiContract()))

	require.NoError(t, handler.Handle(context.Background(), kpiReport(db.KPIVerdictViolated, compensateID)))

	require.Equal(t, compensateID, *deployments.recorded[0].RuleID)
}

// The consequence of removing the DCS-side derivation: a bare (metric, value)
// report — what the shipped target flow sends — is silence about the terms, and
// silence is recorded as silence. Never as compliance.
func TestKPIReportWithoutAVerdictIsRecordedAsNotEvaluated(t *testing.T) {
	handler, deployments, contracts := kpiHandler(t, ptr(kpiContract()))

	require.NoError(t, handler.Handle(context.Background(), kpiReport("", "")))

	require.Len(t, deployments.recorded, 1)
	require.Equal(t, db.KPIVerdictNotEvaluated, deployments.recorded[0].Verdict)
	require.Nil(t, deployments.recorded[0].RuleID)
	require.Zero(t, contracts.reads, "a report that names no rule needs no contract read")
}

// The DCS attributes; it does not invent a referent. An @id that is not among
// the rules this contract deployed cannot be traced back to a term, so the
// report is malformed rather than recorded as a dangling pointer.
func TestKPIReportNamingARuleTheContractDoesNotCarryIsRefused(t *testing.T) {
	handler, deployments, _ := kpiHandler(t, ptr(kpiContract()))

	err := handler.Handle(context.Background(), kpiReport(db.KPIVerdictViolated, "urn:uuid:policy-invented"))

	require.ErrorIs(t, err, ErrKPIRuleUnknown)
	require.Empty(t, deployments.recorded)
}

// A stated conclusion about nothing is equally untraceable.
func TestKPIVerdictWithoutARuleIsRefused(t *testing.T) {
	handler, deployments, _ := kpiHandler(t, ptr(kpiContract()))

	err := handler.Handle(context.Background(), kpiReport(db.KPIVerdictViolated, ""))

	require.ErrorIs(t, err, ErrKPIRuleMissing)
	require.Empty(t, deployments.recorded)
}

func TestKPIReportWithAnUnknownVerdictIsRefused(t *testing.T) {
	handler, deployments, _ := kpiHandler(t, ptr(kpiContract()))

	err := handler.Handle(context.Background(), kpiReport("compliant", availabilityID))

	require.ErrorIs(t, err, ErrKPIVerdictUnknown)
	require.Empty(t, deployments.recorded)
}

// A contract holding no document has no rules to resolve against, and guessing
// is not an option: the report is refused rather than stored unattributed.
func TestKPIReportAgainstAContractWithNoDocumentIsRefused(t *testing.T) {
	handler, deployments, _ := kpiHandler(t, nil)

	err := handler.Handle(context.Background(), kpiReport(db.KPIVerdictViolated, availabilityID))

	require.Error(t, err)
	require.Empty(t, deployments.recorded)
}

// ADR-33 keeps the authorisation path exactly as it was: a verdict is only
// evidence because of who reported it, so a caller that is not the target this
// deployment went to records nothing.
func TestKPIReportFromAnotherTargetIsRefused(t *testing.T) {
	handler, deployments, _ := kpiHandler(t, ptr(kpiContract()))

	report := kpiReport(db.KPIVerdictViolated, availabilityID)
	report.CallerClientID = "dcs-target-t-2"

	err := handler.Handle(context.Background(), report)

	require.ErrorIs(t, err, ErrDeploymentCallbackUnauthorized)
	require.Empty(t, deployments.recorded)
}

func TestKPIReportFromAnUnauthenticatedCallerIsRefused(t *testing.T) {
	handler, deployments, _ := kpiHandler(t, ptr(kpiContract()))

	report := kpiReport(db.KPIVerdictViolated, availabilityID)
	report.CallerClientID = ""

	err := handler.Handle(context.Background(), report)

	require.ErrorIs(t, err, ErrDeploymentCallbackUnauthorized)
	require.Empty(t, deployments.recorded)
}
