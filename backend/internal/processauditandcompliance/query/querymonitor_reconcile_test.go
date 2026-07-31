package qry

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	pacdb "digital-contracting-service/internal/processauditandcompliance/db"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
)

// fakeRiskFindingRepo is an in-memory stand-in for the risk register. It mirrors
// the Postgres repository's contract — Record reports whether the risk is a new
// incident, Resolve closes an open one — so reconcile's decisions can be
// exercised without a database. Whether Postgres upholds that contract is the
// repository's business; what is under test here is what reconcile does with it.
type fakeRiskFindingRepo struct {
	open     map[string]pacdb.RiskFinding
	resolved map[string]bool
}

func newFakeRiskFindingRepo() *fakeRiskFindingRepo {
	return &fakeRiskFindingRepo{
		open:     make(map[string]pacdb.RiskFinding),
		resolved: make(map[string]bool),
	}
}

func fakeKey(f pacdb.RiskFinding) string {
	return f.ContractDID + "|" + f.RiskType + "|" + f.DetailHash
}

func (r *fakeRiskFindingRepo) ListOpen(_ context.Context, _ *sqlx.Tx) ([]pacdb.RiskFinding, error) {
	findings := make([]pacdb.RiskFinding, 0, len(r.open))
	for _, f := range r.open {
		findings = append(findings, f)
	}
	return findings, nil
}

func (r *fakeRiskFindingRepo) Record(_ context.Context, _ *sqlx.Tx, finding pacdb.RiskFinding) (bool, error) {
	key := fakeKey(finding)
	if _, isOpen := r.open[key]; isOpen {
		return false, nil
	}
	r.open[key] = finding
	delete(r.resolved, key)
	return true, nil
}

func (r *fakeRiskFindingRepo) Resolve(_ context.Context, _ *sqlx.Tx, finding pacdb.RiskFinding, _ time.Time) error {
	key := fakeKey(finding)
	delete(r.open, key)
	r.resolved[key] = true
	return nil
}

func riskFor(did string, riskType string, detail string) event2.ComplianceRisk {
	return event2.ComplianceRisk{DID: did, RiskType: riskType, Detail: detail}
}

func riskTypes(risks []event2.ComplianceRisk) []string {
	details := make([]string, 0, len(risks))
	for _, r := range risks {
		details = append(details, r.RiskType+":"+r.Detail)
	}
	return details
}

// A violation must alert when it is first seen — that is the detection the
// requirement is about.
func TestReconcileReportsFirstDetectionAsNew(t *testing.T) {
	monitor := &ComplianceMonitor{FRepo: newFakeRiskFindingRepo()}
	risks := []event2.ComplianceRisk{riskFor("did:a", RiskTypeUnauthorizedAccess, "denied for Acme")}

	newlyDetected, err := monitor.reconcile(context.Background(), nil, risks, time.Now().UTC())
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(newlyDetected) != 1 {
		t.Fatalf("expected the first detection to be reported as new, got %v", riskTypes(newlyDetected))
	}
}

// The sweep runs unattended every few minutes. A violation that still holds must
// not alert again, or the audit trail fills with repetitions of one incident and
// the moment of detection becomes unfindable.
func TestReconcileStaysSilentWhileRiskStillHolds(t *testing.T) {
	monitor := &ComplianceMonitor{FRepo: newFakeRiskFindingRepo()}
	risks := []event2.ComplianceRisk{riskFor("did:a", RiskTypeUnauthorizedAccess, "denied for Acme")}
	ctx := context.Background()

	if _, err := monitor.reconcile(ctx, nil, risks, time.Now().UTC()); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	newlyDetected, err := monitor.reconcile(ctx, nil, risks, time.Now().UTC())
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if len(newlyDetected) != 0 {
		t.Fatalf("expected no alert for a risk that was already on record, got %v", riskTypes(newlyDetected))
	}
}

// One contract can carry several risks of the same type at once — one per
// outstanding approver, one per denied actor. Each is its own violation and each
// must alert; keying the register by risk type alone would swallow all but the
// first.
func TestReconcileAlertsPerViolationNotPerRiskType(t *testing.T) {
	monitor := &ComplianceMonitor{FRepo: newFakeRiskFindingRepo()}
	ctx := context.Background()
	checkedAt := time.Now().UTC()

	first := []event2.ComplianceRisk{riskFor("did:a", RiskTypeUnauthorizedAccess, "denied for Acme")}
	if _, err := monitor.reconcile(ctx, nil, first, checkedAt); err != nil {
		t.Fatalf("first sweep: %v", err)
	}

	second := []event2.ComplianceRisk{
		riskFor("did:a", RiskTypeUnauthorizedAccess, "denied for Acme"),
		riskFor("did:a", RiskTypeUnauthorizedAccess, "denied for Globex"),
	}
	newlyDetected, err := monitor.reconcile(ctx, nil, second, checkedAt)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if len(newlyDetected) != 1 || newlyDetected[0].Detail != "denied for Globex" {
		t.Fatalf("expected only the second actor's denial to alert, got %v", riskTypes(newlyDetected))
	}
}

// A risk the sweep no longer sees has been dealt with — the approval was given,
// the deployment succeeded. Leaving it open would make a later recurrence look
// like the same, still-unresolved incident.
func TestReconcileClosesFindingsTheSweepNoLongerDetects(t *testing.T) {
	repo := newFakeRiskFindingRepo()
	monitor := &ComplianceMonitor{FRepo: repo}
	ctx := context.Background()
	risks := []event2.ComplianceRisk{riskFor("did:a", RiskTypeMissingApproval, "missing approval from Bob")}

	if _, err := monitor.reconcile(ctx, nil, risks, time.Now().UTC()); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if _, err := monitor.reconcile(ctx, nil, nil, time.Now().UTC()); err != nil {
		t.Fatalf("second sweep: %v", err)
	}

	if len(repo.open) != 0 {
		t.Fatalf("expected the finding to be closed once the risk was gone, still open: %v", repo.open)
	}
	if len(repo.resolved) != 1 {
		t.Fatalf("expected exactly one resolved finding, got %v", repo.resolved)
	}
}

// A violation that returns after being resolved is a new incident and must alert
// again — silence here would mean a contract falls out of compliance a second
// time with nobody told.
func TestReconcileAlertsAgainWhenAResolvedRiskRecurs(t *testing.T) {
	monitor := &ComplianceMonitor{FRepo: newFakeRiskFindingRepo()}
	ctx := context.Background()
	risks := []event2.ComplianceRisk{riskFor("did:a", RiskTypeMissingApproval, "missing approval from Bob")}

	if _, err := monitor.reconcile(ctx, nil, risks, time.Now().UTC()); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if _, err := monitor.reconcile(ctx, nil, nil, time.Now().UTC()); err != nil {
		t.Fatalf("clearing sweep: %v", err)
	}
	newlyDetected, err := monitor.reconcile(ctx, nil, risks, time.Now().UTC())
	if err != nil {
		t.Fatalf("recurrence sweep: %v", err)
	}
	if len(newlyDetected) != 1 {
		t.Fatalf("expected the recurrence to alert as a new incident, got %v", riskTypes(newlyDetected))
	}
}

// Two risks are the same violation only if their detail matches. The details are
// built from persisted facts (the approver, the actor, the metric), never from
// the sweep's own clock, or every sweep would hash differently and alert afresh.
func TestFindingIdentityIsStableAcrossSweeps(t *testing.T) {
	risk := riskFor("did:a", RiskTypeUnderperformance, `reported KPI "uptime" = "97%" violates`)

	first := findingOf(risk, time.Now().UTC())
	second := findingOf(risk, time.Now().UTC().Add(5*time.Minute))

	if first.DetailHash != second.DetailHash {
		t.Fatalf("the same violation hashed differently across sweeps: %s vs %s", first.DetailHash, second.DetailHash)
	}
	if first.DetailHash == findingOf(riskFor("did:a", RiskTypeUnderperformance, "something else"), time.Now().UTC()).DetailHash {
		t.Fatal("two different violations hashed the same, so one would never alert")
	}
}
