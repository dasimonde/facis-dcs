package machineidentity

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

const columns = `id, name, oauth_client_id, participant_did, roles, description, enabled, secret_issued_at, created_by, created_at, updated_at`

// PostgresRepo is the machine identity registry backed by the DCS database.
type PostgresRepo struct {
	DB *sqlx.DB
}

func NewPostgresRepo(db *sqlx.DB) *PostgresRepo {
	return &PostgresRepo{DB: db}
}

func (r *PostgresRepo) List(ctx context.Context) ([]Identity, error) {
	query := `SELECT ` + columns + ` FROM machine_identities ORDER BY name ASC`
	identities := []Identity{}
	if err := r.DB.SelectContext(ctx, &identities, query); err != nil {
		return nil, fmt.Errorf("list machine identities: %w", err)
	}
	return identities, nil
}

func (r *PostgresRepo) Read(ctx context.Context, id string) (*Identity, error) {
	query := `SELECT ` + columns + ` FROM machine_identities WHERE id = $1`
	var identity Identity
	if err := r.DB.GetContext(ctx, &identity, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("read machine identity %s: %w", id, err)
	}
	return &identity, nil
}

func (r *PostgresRepo) FindByClientID(ctx context.Context, clientID string) (*Identity, error) {
	query := `SELECT ` + columns + ` FROM machine_identities WHERE oauth_client_id = $1`
	var identity Identity
	if err := r.DB.GetContext(ctx, &identity, query, clientID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("resolve machine identity for client %q: %w", clientID, err)
	}
	return &identity, nil
}

func (r *PostgresRepo) Create(ctx context.Context, data Identity) (*Identity, error) {
	const statement = `
        INSERT INTO machine_identities
            (name, oauth_client_id, participant_did, roles, description, enabled, secret_issued_at, created_by)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        RETURNING ` + columns
	var created Identity
	if err := r.DB.GetContext(ctx, &created, statement,
		data.Name, data.OAuthClientID, data.ParticipantDID, data.RolesJSON,
		data.Description, data.Enabled, data.SecretIssuedAt, data.CreatedBy,
	); err != nil {
		return nil, fmt.Errorf("create machine identity %q: %w", data.Name, err)
	}
	return &created, nil
}

func (r *PostgresRepo) Update(ctx context.Context, data Identity) (*Identity, error) {
	const statement = `
        UPDATE machine_identities
        SET name = $2, participant_did = $3, roles = $4, description = $5,
            enabled = $6, updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
        RETURNING ` + columns
	var updated Identity
	if err := r.DB.GetContext(ctx, &updated, statement,
		data.ID, data.Name, data.ParticipantDID, data.RolesJSON, data.Description, data.Enabled,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update machine identity %s: %w", data.ID, err)
	}
	return &updated, nil
}

func (r *PostgresRepo) Delete(ctx context.Context, id string) error {
	if _, err := r.DB.ExecContext(ctx, `DELETE FROM machine_identities WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete machine identity %s: %w", id, err)
	}
	return nil
}

func (r *PostgresRepo) DeleteByClientIDTx(ctx context.Context, tx *sqlx.Tx, clientID string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM machine_identities WHERE oauth_client_id = $1`, clientID); err != nil {
		return fmt.Errorf("withdraw the authority of machine client %q: %w", clientID, err)
	}
	return nil
}

func (r *PostgresRepo) TouchSecretIssuedAt(ctx context.Context, id string, at time.Time) error {
	const statement = `UPDATE machine_identities SET secret_issued_at = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1`
	if _, err := r.DB.ExecContext(ctx, statement, id, at); err != nil {
		return fmt.Errorf("record the issued secret for machine identity %s: %w", id, err)
	}
	return nil
}

// execer is the write surface shared by the connection pool and a transaction,
// so one statement serves both instead of two that have to agree.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}

// Upsert keys on oauth_client_id: a seeded identity is identified by the client
// it authenticates as, and re-seeding must not duplicate it. Roles and the
// attributed DID are refreshed so the declared configuration stays the source
// of truth for seeded entries.
func (r *PostgresRepo) Upsert(ctx context.Context, data Identity) error {
	return upsert(ctx, r.DB, data)
}

// UpsertTx registers an identity inside the caller's transaction, so it commits
// with whatever else had to be true for the credential to be usable.
func (r *PostgresRepo) UpsertTx(ctx context.Context, tx *sqlx.Tx, data Identity) error {
	return upsert(ctx, tx, data)
}

func upsert(ctx context.Context, ex execer, data Identity) error {
	// secret_issued_at is coalesced rather than assigned: a seed carries no
	// secret (the client's comes from the same deployment configuration) and
	// must not erase the issue date of a credential handed out since.
	const statement = `
        INSERT INTO machine_identities
            (name, oauth_client_id, participant_did, roles, description, enabled, created_by, secret_issued_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        ON CONFLICT (oauth_client_id) DO UPDATE
        SET participant_did = EXCLUDED.participant_did,
            roles = EXCLUDED.roles,
            secret_issued_at = COALESCE(EXCLUDED.secret_issued_at, machine_identities.secret_issued_at),
            updated_at = CURRENT_TIMESTAMP
    `
	if _, err := ex.ExecContext(ctx, statement,
		data.Name, data.OAuthClientID, data.ParticipantDID, data.RolesJSON,
		data.Description, data.Enabled, data.CreatedBy, data.SecretIssuedAt,
	); err != nil {
		return fmt.Errorf("seed machine identity %q: %w", data.Name, err)
	}
	return nil
}
