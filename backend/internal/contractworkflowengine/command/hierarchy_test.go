package command

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
)

// cycleRepoFake resolves a parent chain from an in-memory did->parentDID map.
type cycleRepoFake struct {
	db.ContractRepo
	parents map[string]string // did -> parentDID ("" = no parent)
	known   map[string]bool   // dids that resolve locally
}

func (r *cycleRepoFake) ReadDataByDID(_ context.Context, _ *sqlx.Tx, did string) (*db.Contract, error) {
	if !r.known[did] {
		return nil, fmt.Errorf("contract with DID %s not found", did)
	}
	c := &db.Contract{DID: did}
	if parent := r.parents[did]; parent != "" {
		data, err := datatype.NewJSON(map[string]any{
			"dcs:parentContract": map[string]any{"@id": parent},
		})
		if err != nil {
			return nil, err
		}
		c.ContractData = &data
	}
	return c, nil
}

func TestCheckNoParentCycle(t *testing.T) {
	// A <- B (B's parent is A). Updating A to reference B closes the loop.
	repo := &cycleRepoFake{
		parents: map[string]string{"B": "A"},
		known:   map[string]bool{"A": true, "B": true},
	}

	if err := checkNoParentCycle(context.Background(), nil, repo, "A", "B"); !errors.Is(err, ErrContractHierarchyCycle) {
		t.Fatalf("expected cycle error, got %v", err)
	}

	// Setting B's parent to A when A has no parent is fine.
	repoNoLoop := &cycleRepoFake{
		parents: map[string]string{},
		known:   map[string]bool{"A": true, "B": true},
	}
	if err := checkNoParentCycle(context.Background(), nil, repoNoLoop, "B", "A"); err != nil {
		t.Fatalf("expected no cycle, got %v", err)
	}

	// A cross-instance parent (not resolvable locally) ends the walk cleanly.
	repoRemote := &cycleRepoFake{
		parents: map[string]string{},
		known:   map[string]bool{"child": true},
	}
	if err := checkNoParentCycle(context.Background(), nil, repoRemote, "child", "did:remote:frame"); err != nil {
		t.Fatalf("expected non-local parent to be tolerated, got %v", err)
	}
}

func TestExtractParentContractDID(t *testing.T) {
	object, _ := datatype.NewJSON(map[string]any{"dcs:parentContract": map[string]any{"@id": "did:example:p"}})
	if got := extractParentContractDID(&object); got != "did:example:p" {
		t.Fatalf("object form: got %q", got)
	}

	array, _ := datatype.NewJSON(map[string]any{"dcs:parentContract": []any{map[string]any{"@id": "did:example:q"}}})
	if got := extractParentContractDID(&array); got != "did:example:q" {
		t.Fatalf("array form: got %q", got)
	}

	none, _ := datatype.NewJSON(map[string]any{"@type": "dcs:Contract"})
	if got := extractParentContractDID(&none); got != "" {
		t.Fatalf("no parent: got %q", got)
	}

	if got := extractParentContractDID(nil); got != "" {
		t.Fatalf("nil: got %q", got)
	}
}
