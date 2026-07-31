// Package machineidentity is the registry of non-human callers: the SRS Table 5
// System Users and the Contract Target Systems. Each authenticates with its own
// OAuth2 client provisioned in Hydra, so a credential can be issued once, shown
// once and rotated without a redeploy (ADR-27).
package machineidentity

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/jmoiron/sqlx"
)

// Identity is one registered machine caller.
//
// The secret is deliberately absent: Hydra holds it hashed and cannot return
// it, which is what makes "shown once" a property of the system rather than a
// promise. SecretIssuedAt only lets an operator see how old a credential is.
type Identity struct {
	ID             string     `db:"id"`
	Name           string     `db:"name"`
	OAuthClientID  string     `db:"oauth_client_id"`
	ParticipantDID string     `db:"participant_did"`
	RolesJSON      string     `db:"roles"`
	Description    *string    `db:"description"`
	Enabled        bool       `db:"enabled"`
	SecretIssuedAt *time.Time `db:"secret_issued_at"`
	CreatedBy      string     `db:"created_by"`
	CreatedAt      time.Time  `db:"created_at"`
	UpdatedAt      time.Time  `db:"updated_at"`
}

// Roles decodes the stored role list. Authority is read from here by client_id
// and never from the token, so a caller cannot widen its own reach.
func (i Identity) Roles() ([]string, error) {
	trimmed := strings.TrimSpace(i.RolesJSON)
	if trimmed == "" {
		return nil, nil
	}
	var roles []string
	if err := json.Unmarshal([]byte(trimmed), &roles); err != nil {
		return nil, fmt.Errorf("machine identity %q has an unreadable role list: %w", i.Name, err)
	}
	return roles, nil
}

// EncodeRoles renders a role list for storage.
func EncodeRoles(roles []string) (string, error) {
	if len(roles) == 0 {
		return "", fmt.Errorf("a machine identity needs at least one role")
	}
	encoded, err := json.Marshal(roles)
	if err != nil {
		return "", fmt.Errorf("could not encode the role list: %w", err)
	}
	return string(encoded), nil
}

// ContractTargetCredential is the credential issued to one Contract Target
// System, from which the registry row that makes it usable is derived.
//
// A target authenticates its deployment callbacks as its own OAuth2 client, and
// a machine caller's roles are resolved from this registry by client id and
// never from its token. A client with no row therefore authenticates and is
// then refused at the scope gate: the acknowledgement never lands and the
// contract stays SIGNED with nothing said about why (ADR-27).
type ContractTargetCredential struct {
	// ClientID is the OAuth2 client provisioned in Hydra for this target.
	ClientID string
	// TargetName names the registry entry the credential belongs to, so an
	// operator reading the machine identity list can tell which target a
	// generated client id stands for.
	TargetName string
	// ParticipantDID attributes the callbacks to this deployment: a target acts
	// on its behalf, exactly as the declaratively seeded system clients do.
	ParticipantDID string
	// IssuedBy records who asked for the credential.
	IssuedBy string
	IssuedAt time.Time
}

// Identity renders the registry row for the credential.
//
// The shape matches what a target declared in deployment configuration gets
// from the DCS_SYSTEM_CLIENTS seed (cmd/dcs/machine_identity_seed.go): named
// after the client it authenticates as, attributed to this deployment, enabled,
// and holding the single Contract Target System role. The two paths must
// produce the same row or only one of them will ever authorise a callback.
func (c ContractTargetCredential) Identity() (Identity, error) {
	clientID := strings.TrimSpace(c.ClientID)
	if clientID == "" {
		return Identity{}, fmt.Errorf("a contract target credential needs the OAuth2 client it authenticates as")
	}
	participantDID := strings.TrimSpace(c.ParticipantDID)
	if participantDID == "" {
		return Identity{}, fmt.Errorf("contract target credential %q needs a participant DID to attribute its callbacks to", clientID)
	}

	roles, err := EncodeRoles([]string{userrole.ContractTargetSystem.String()})
	if err != nil {
		return Identity{}, err
	}

	description := fmt.Sprintf("Callback credential of contract target %q.", strings.TrimSpace(c.TargetName))
	createdBy := strings.TrimSpace(c.IssuedBy)
	if createdBy == "" {
		createdBy = "contract target registry"
	}
	identity := Identity{
		Name:           clientID,
		OAuthClientID:  clientID,
		ParticipantDID: participantDID,
		RolesJSON:      roles,
		Description:    &description,
		Enabled:        true,
		CreatedBy:      createdBy,
	}
	if !c.IssuedAt.IsZero() {
		issuedAt := c.IssuedAt.UTC()
		identity.SecretIssuedAt = &issuedAt
	}
	return identity, nil
}

// Repo stores registered machine identities.
type Repo interface {
	List(ctx context.Context) ([]Identity, error)
	Read(ctx context.Context, id string) (*Identity, error)
	// FindByClientID resolves the caller behind an access token's client_id
	// claim. It is on the hot path of every machine-authenticated request.
	FindByClientID(ctx context.Context, clientID string) (*Identity, error)
	Create(ctx context.Context, data Identity) (*Identity, error)
	Update(ctx context.Context, data Identity) (*Identity, error)
	Delete(ctx context.Context, id string) error
	// TouchSecretIssuedAt records that a new secret was handed out.
	TouchSecretIssuedAt(ctx context.Context, id string, at time.Time) error
	// Upsert seeds a declaratively configured identity, so a deployment can
	// bootstrap the callers it needs before anyone logs in to create them.
	Upsert(ctx context.Context, data Identity) error
	// UpsertTx is Upsert inside a caller's transaction, for a registration that
	// has to stand or fall with another write — a contract target's client id
	// and the row that says what that client may do.
	UpsertTx(ctx context.Context, tx *sqlx.Tx, data Identity) error
	// DeleteByClientIDTx withdraws the authority of a client whose reason to
	// exist is gone, in the same transaction that removes it. It is keyed on
	// the client rather than the row id because the caller knows the client the
	// deleted thing authenticated as, not the registry entry behind it.
	DeleteByClientIDTx(ctx context.Context, tx *sqlx.Tx, clientID string) error
}
