package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
)

// stubTargetRepo serves one registry entry; every other method is unused by the
// authorisation path and panics if the check ever reaches for more than it needs.
type stubTargetRepo struct {
	target *db.ContractTarget
}

func (s *stubTargetRepo) ReadTarget(_ context.Context, _ *sqlx.Tx, _ string) (*db.ContractTarget, error) {
	return s.target, nil
}

func (s *stubTargetRepo) ListTargets(context.Context, *sqlx.Tx) ([]db.ContractTarget, error) {
	panic("not used by the authorisation path")
}
func (s *stubTargetRepo) CreateTarget(context.Context, *sqlx.Tx, db.ContractTarget) (*db.ContractTarget, error) {
	panic("not used by the authorisation path")
}
func (s *stubTargetRepo) UpdateTarget(context.Context, *sqlx.Tx, db.ContractTarget) (*db.ContractTarget, error) {
	panic("not used by the authorisation path")
}
func (s *stubTargetRepo) DeleteTarget(context.Context, *sqlx.Tx, string) error {
	panic("not used by the authorisation path")
}
func (s *stubTargetRepo) CountContractsDesignating(context.Context, *sqlx.Tx, string) (int, error) {
	panic("not used by the authorisation path")
}
func (s *stubTargetRepo) DesignateForContract(context.Context, *sqlx.Tx, string, *string) (bool, error) {
	panic("not used by the authorisation path")
}
func (s *stubTargetRepo) SetCredential(context.Context, *sqlx.Tx, string, string, time.Time) error {
	panic("not used by the authorisation path")
}

func ptr[T any](v T) *T { return &v }

func deploymentTo(targetID string) *db.ContractDeployment {
	return &db.ContractDeployment{CorrelationID: "corr-1", TargetID: ptr(targetID)}
}

// The point of replacing the shared secret: a callback is accepted only from
// the target the deployment actually went to.
func TestCallbackAcceptsTheTargetItWasDispatchedTo(t *testing.T) {
	handler := DeploymentCallbackHandler{TargetRepo: &stubTargetRepo{
		target: &db.ContractTarget{ID: "t-1", OAuthClientID: ptr("dcs-target-t-1")},
	}}

	if err := handler.authorizeCaller(context.Background(), nil, deploymentTo("t-1"), "dcs-target-t-1"); err != nil {
		t.Fatalf("the dispatched target was refused: %v", err)
	}
}

// A different registered target holds a valid credential of its own. Under one
// shared secret this was indistinguishable; now it is refused.
func TestCallbackRefusesADifferentTarget(t *testing.T) {
	handler := DeploymentCallbackHandler{TargetRepo: &stubTargetRepo{
		target: &db.ContractTarget{ID: "t-1", OAuthClientID: ptr("dcs-target-t-1")},
	}}

	err := handler.authorizeCaller(context.Background(), nil, deploymentTo("t-1"), "dcs-target-t-2")
	if !errors.Is(err, ErrDeploymentCallbackUnauthorized) {
		t.Fatalf("another target was allowed to acknowledge this deployment: %v", err)
	}
}

// With no credential issued there is nothing to check the caller against, so
// accepting it would restore the "some target said so" property.
func TestCallbackRefusesATargetWithNoCredential(t *testing.T) {
	handler := DeploymentCallbackHandler{TargetRepo: &stubTargetRepo{
		target: &db.ContractTarget{ID: "t-1"},
	}}

	err := handler.authorizeCaller(context.Background(), nil, deploymentTo("t-1"), "dcs-target-t-1")
	if !errors.Is(err, ErrDeploymentCallbackUnauthorized) {
		t.Fatalf("a target with no issued credential was accepted: %v", err)
	}
}

func TestCallbackRefusesADeploymentWithNoTarget(t *testing.T) {
	handler := DeploymentCallbackHandler{TargetRepo: &stubTargetRepo{}}

	err := handler.authorizeCaller(context.Background(), nil, &db.ContractDeployment{CorrelationID: "corr-1"}, "dcs-target-t-1")
	if !errors.Is(err, ErrDeploymentCallbackUnauthorized) {
		t.Fatalf("a deployment with no recorded target was acknowledged: %v", err)
	}
}
