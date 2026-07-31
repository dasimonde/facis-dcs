package command

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
)

// SeedTarget is one Contract Target System declared in deployment
// configuration (ADR-25). A fresh install has an empty registry, so without
// this an operator has to open the admin UI before any contract can be pointed
// anywhere — including in a test cluster that is torn down every run.
type SeedTarget struct {
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Description *string `json:"description,omitempty"`
	// Enabled defaults to true: a target declared in configuration is one the
	// deployment intends to use.
	Enabled *bool `json:"enabled,omitempty"`
	// OAuthClientID names the client this target authenticates its callbacks
	// as, for the case where the client is provisioned by the same deployment
	// configuration rather than issued through the admin UI (ADR-27). Left
	// empty, the target has no credential until an administrator issues one and
	// cannot acknowledge a deployment before then.
	OAuthClientID string `json:"oauth_client_id,omitempty"`
}

// ParseSeedTargets decodes the configured target list.
func ParseSeedTargets(raw []byte) ([]SeedTarget, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, nil
	}
	var entries []SeedTarget
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("parse configured contract targets: %w", err)
	}
	return entries, nil
}

type listTargetsFn func(ctx context.Context) ([]db.ContractTarget, error)
type createTargetFn func(ctx context.Context, target db.ContractTarget) error

// seedTargets registers each configured target that is not already present,
// matching on name, and returns how many it added.
//
// It deliberately never updates an existing entry. Seeding runs on every start,
// so overwriting would silently revert an administrator who repointed a target
// in the UI — deployments would go somewhere they had been explicitly moved
// away from, with the configuration winning every restart.
func seedTargets(ctx context.Context, list listTargetsFn, create createTargetFn, entries []SeedTarget) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	existing, err := list(ctx)
	if err != nil {
		return 0, fmt.Errorf("read registered contract targets: %w", err)
	}
	known := make(map[string]bool, len(existing))
	for _, target := range existing {
		known[target.Name] = true
	}

	seeded := 0
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		url := strings.TrimSpace(entry.URL)
		if name == "" || url == "" {
			return seeded, fmt.Errorf("configured contract target needs both a name and a url, got name=%q url=%q", name, url)
		}
		if known[name] {
			continue
		}
		enabled := true
		if entry.Enabled != nil {
			enabled = *entry.Enabled
		}
		target := db.ContractTarget{
			Name:        name,
			URL:         url,
			Description: entry.Description,
			Enabled:     enabled,
			CreatedBy:   "system:deployment-configuration",
		}
		if clientID := strings.TrimSpace(entry.OAuthClientID); clientID != "" {
			target.OAuthClientID = &clientID
		}
		if err := create(ctx, target); err != nil {
			return seeded, fmt.Errorf("register configured contract target %q: %w", name, err)
		}
		known[name] = true
		seeded++
	}
	return seeded, nil
}

// SeedContractTargets registers the targets declared in deployment
// configuration, using the given repository. Returns how many were added.
func SeedContractTargets(ctx context.Context, dbx *sqlx.DB, repo db.ContractTargetRepo, entries []SeedTarget) (int, error) {
	if len(entries) == 0 {
		return 0, nil
	}
	tx, err := dbx.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	seeded, err := seedTargets(ctx,
		func(ctx context.Context) ([]db.ContractTarget, error) { return repo.ListTargets(ctx, tx) },
		func(ctx context.Context, target db.ContractTarget) error {
			_, err := repo.CreateTarget(ctx, tx, target)
			return err
		},
		entries)
	if err != nil {
		return seeded, err
	}
	if err := tx.Commit(); err != nil {
		return seeded, err
	}
	return seeded, nil
}
