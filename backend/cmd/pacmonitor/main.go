// Command pacmonitor runs one continuous-compliance-monitoring sweep and exits
// (DCS-FR-PACM-02, "the system MUST continuously monitor contract lifecycle
// activities for violations of defined compliance rules"). It is the scheduled
// counterpart of GET /pac/monitor: the same detection logic, driven by a
// CronJob instead of a request, so a violation is found without anyone asking.
//
// It deliberately opens nothing but the database. The DCS server's startup path
// gates on the HSM self-test, Federated Catalogue readiness and the status-list
// probe, and reusing it here would make compliance monitoring stop precisely
// when a neighbouring component is in trouble — the moment it is needed most.
//
// It needs no event-bus connection either. Detections are written to the
// transactional outbox in the same transaction that records them; the running
// DCS server's OutboxProcessor is what anchors them in the audit trail and
// fans them out over NATS to webhook subscribers.
//
// A detected risk is not a job failure: the sweep reports risks on stdout and
// exits 0. A non-zero exit means the sweep could not run, which is the only
// condition an operator should be paged about.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	cwerepo "digital-contracting-service/internal/contractworkflowengine/db/pg"
	pacrepo "digital-contracting-service/internal/processauditandcompliance/db/pg"
	qry "digital-contracting-service/internal/processauditandcompliance/query"
)

func main() {
	timeout := flag.Duration("timeout", 2*time.Minute, "maximum duration of one sweep")
	flag.Parse()

	if err := run(*timeout); err != nil {
		fmt.Fprintf(os.Stderr, "pacmonitor: %v\n", err)
		os.Exit(1)
	}
}

func run(timeout time.Duration) error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	db, err := sqlx.Connect("postgres", databaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	approvalTaskRepo := cwerepo.PostgresApprovalTaskRepo{}
	contractRepo := cwerepo.PostgresContractRepo{}
	riskFindingRepo := pacrepo.PostgresRiskFindingRepo{}

	monitor := qry.ComplianceMonitor{
		DB:     db,
		ATRepo: &approvalTaskRepo,
		CRepo:  &contractRepo,
		FRepo:  &riskFindingRepo,
	}

	result, err := monitor.Handle(ctx, qry.MonitorQry{MonitoredBy: qry.SystemMonitorPrincipal})
	if err != nil {
		return fmt.Errorf("sweep failed: %w", err)
	}

	fmt.Printf("pacmonitor: swept at %s, %d risk(s) currently open\n",
		result.CheckedAt.Format(time.RFC3339), len(result.Risks))
	for _, risk := range result.Risks {
		fmt.Printf("pacmonitor: %s %s: %s\n", risk.RiskType, risk.DID, risk.Detail)
	}
	return nil
}
