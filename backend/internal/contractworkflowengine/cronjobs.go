// Package contractworkflowengine implements the core contract lifecycle:
// creation, negotiation, review, approval, termination, expiry and
// renewal-deadline notices (this file's CronJob). It follows the
// repository-wide CQRS layout (command/, query/, db/, datatype/, event/),
// plus two contract-specific additions: negotiationmerging (folds accepted
// change requests into a new contract version) and remotesync (the
// command/RPC side of DCS-to-DCS federation; the peer-transport side lives
// in the separate dcstodcs package).
//
// Each DCS instance writes its own copy of a contract (ADR-13), so each
// instance expires its own copy. Expiry is exposed instantly to readers via
// the contracts_effective DB view (EXPIRED computed at query time), while
// this file's cron job lags behind to persist the state and emit the
// corresponding event/audit-trail entry.
package contractworkflowengine

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/conf"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	database "digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/pdfgeneration/statuspublication"
)

type CronJob struct {
	DB    *sqlx.DB
	CRepo database.ContractRepo
}

func (j CronJob) Start(ctx context.Context, db *sqlx.DB) {
	go startExpiryScheduler(ctx, db, j.CRepo, conf.ExpirationCronJobTimeOut())
}

// contractTask is one cron step. Each tick reads the contracts in one
// transaction, then handles each contract in its own transaction.
type contractTask interface {
	read(ctx context.Context, tx *sqlx.Tx) ([]database.ContractMetadata, error)
	handle(ctx context.Context, tx *sqlx.Tx, c database.ContractMetadata) error
}

func startExpiryScheduler(ctx context.Context, db *sqlx.DB, repo database.ContractRepo, interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		runTask(ctx, db, "expired", expiryTask{repo})
		runTask(ctx, db, "renewal-due", renewalDueTask{repo})
	}
}

func runTask(ctx context.Context, db *sqlx.DB, name string, t contractTask) {
	var contracts []database.ContractMetadata
	err := inTx(ctx, db, func(tx *sqlx.Tx) error {
		var err error
		contracts, err = t.read(ctx, tx)
		return err
	})
	if err != nil {
		log.Printf("could not read %s contracts: %v", name, err)
		return
	}
	if len(contracts) > 0 {
		log.Printf("%d %s contracts", len(contracts), name)
	}
	for _, c := range contracts {
		if err := inTx(ctx, db, func(tx *sqlx.Tx) error { return t.handle(ctx, tx, c) }); err != nil {
			log.Printf("could not handle %s contract: %v", name, err)
		}
	}
}

func inTx(ctx context.Context, db *sqlx.DB, fn func(*sqlx.Tx) error) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}
	return nil
}

type expiryTask struct{ repo database.ContractRepo }

func (t expiryTask) read(ctx context.Context, tx *sqlx.Tx) ([]database.ContractMetadata, error) {
	return t.repo.ReadExpiredContracts(ctx, tx)
}

// handle persists EXPIRED and emits ContractExpired. The event carries the
// configured policy, so subscribers start the renewal, termination or
// archiving workflow (DCS-FR-CWE-10). A contract without a policy expires too.
func (t expiryTask) handle(ctx context.Context, tx *sqlx.Tx, expiredContract database.ContractMetadata) error {
	evt, err := newExpiredEvent(expiredContract, time.Now().UTC())
	if err != nil {
		return err
	}
	// Readers already see EXPIRED instantly via the contracts_effective view once
	// exp_date has passed; this persists that state physically and emits the
	// ContractExpired event, so it necessarily lags the view by up to the poll interval.
	// Several replicas run this task: only the one whose update changes the row
	// emits the event.
	expired, err := t.repo.ExpireContract(ctx, tx, expiredContract.DID)
	if err != nil {
		return fmt.Errorf("could not update expired contract with DID %s: %w", expiredContract.DID, err)
	}
	if !expired {
		return nil
	}
	if err := event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine); err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}
	// The expiry reaches the status list in the same transaction as the state
	// (DCS-OR-C2PA-005), as termination and revocation do.
	return statuspublication.EnqueueTx(ctx, tx, expiredContract.DID, contractstate.Expired.String(),
		"Contract reached expiration date", evt.OccurredAt)
}

func newExpiredEvent(c database.ContractMetadata, now time.Time) (contractevents.ContractExpired, error) {
	state, err := contractstate.NewContractState(c.State)
	if err != nil {
		return contractevents.ContractExpired{}, fmt.Errorf("could not create state for expired contract with DID %s: %w", c.DID, err)
	}
	var policy *expirationpolicy.ExpirationPolicy
	if c.ExpPolicy != nil {
		p, err := expirationpolicy.NewExpirationPolicy(*c.ExpPolicy)
		if err != nil {
			return contractevents.ContractExpired{}, fmt.Errorf("could not create expiration policy: %w", err)
		}
		policy = &p
	}
	return contractevents.ContractExpired{
		DID:             c.DID,
		ContractVersion: c.ContractVersion,
		State:           state,
		ExpPolicy:       policy,
		OccurredAt:      now,
	}, nil
}

type renewalDueTask struct{ repo database.ContractRepo }

func (t renewalDueTask) read(ctx context.Context, tx *sqlx.Tx) ([]database.ContractMetadata, error) {
	return t.repo.ReadRenewalDueContracts(ctx, tx)
}

// The ledger claim and the event share one transaction: a failed event
// write releases the claim, so the next tick retries the notice.
func (t renewalDueTask) handle(ctx context.Context, tx *sqlx.Tx, c database.ContractMetadata) error {
	evt, err := newRenewalDueEvent(c, time.Now().UTC())
	if err != nil {
		return err
	}
	claimed, err := t.repo.ClaimRenewalNotice(ctx, tx, evt.DID, evt.ExpDate)
	if err != nil {
		return fmt.Errorf("could not record renewal notice for contract with DID %s: %w", c.DID, err)
	}
	if !claimed {
		return nil
	}
	if err := event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine); err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}
	return nil
}

func newRenewalDueEvent(c database.ContractMetadata, now time.Time) (contractevents.ContractRenewalDue, error) {
	if c.ExpDate == nil || c.ExpNoticePeriod == nil {
		return contractevents.ContractRenewalDue{}, fmt.Errorf("renewal-due contract with DID %s has no expiration date or notice period", c.DID)
	}
	state, err := contractstate.NewContractState(c.State)
	if err != nil {
		return contractevents.ContractRenewalDue{}, fmt.Errorf("could not create state for contract with DID %s: %w", c.DID, err)
	}
	var policy *expirationpolicy.ExpirationPolicy
	if c.ExpPolicy != nil {
		p, err := expirationpolicy.NewExpirationPolicy(*c.ExpPolicy)
		if err != nil {
			return contractevents.ContractRenewalDue{}, fmt.Errorf("could not create expiration policy: %w", err)
		}
		policy = &p
	}
	return contractevents.ContractRenewalDue{
		DID:             c.DID,
		ContractVersion: c.ContractVersion,
		State:           state,
		ExpDate:         *c.ExpDate,
		ExpNoticePeriod: *c.ExpNoticePeriod,
		ExpPolicy:       policy,
		Responsible:     c.Responsible,
		OccurredAt:      now,
	}, nil
}
