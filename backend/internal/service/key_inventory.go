package service

import (
	"context"
	"time"

	keyinventory "digital-contracting-service/gen/key_inventory"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/hsm"

	"github.com/jmoiron/sqlx"
)

// KeyInventory service implementation: a read-only view over the HSM key
// labels and their active versions from pki_active_key_version.
type keyInventorysrvc struct {
	DB *sqlx.DB
	auth.JWTAuthenticator
}

// NewKeyInventory returns the KeyInventory service implementation.
func NewKeyInventory(db *sqlx.DB, jwtAuth auth.JWTAuthenticator) keyinventory.Service {
	return &keyInventorysrvc{DB: db, JWTAuthenticator: jwtAuth}
}

type activeKeyVersionRow struct {
	Label         string    `db:"label"`
	ActiveVersion int       `db:"active_version"`
	UpdatedAt     time.Time `db:"updated_at"`
}

// List returns the per-purpose HSM key labels with their active version. A
// label without a pki_active_key_version row is at version 1: the initial
// token provisioning creates the un-suffixed key and writes no version row.
func (s *keyInventorysrvc) List(ctx context.Context, p *keyinventory.ListPayload) (*keyinventory.KeyInventoryResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	var rows []activeKeyVersionRow
	if err := s.DB.SelectContext(ctx, &rows,
		`SELECT label, active_version, updated_at FROM pki_active_key_version`); err != nil {
		return nil, keyinventory.MakeInternalError(err)
	}
	versions := make(map[string]activeKeyVersionRow, len(rows))
	for _, row := range rows {
		versions[row.Label] = row
	}

	inventory := []struct{ label, purpose string }{
		{hsm.KeyLabelDID(), "DID document signing (did:web verification method)"},
		{hsm.KeyLabelVC(), "Lifecycle verifiable-credential signing"},
		{hsm.KeyLabelJAR(), "OpenID4VP JAR request signing"},
		{hsm.KeyLabelC2PA(), "C2PA manifest (COSE) signing"},
		{hsm.KeyLabelECDH(), "Content-encryption key agreement (ECDH wrap of per-contract CEKs)"},
	}

	keys := make([]*keyinventory.HSMKeyInfo, 0, len(inventory))
	for _, entry := range inventory {
		info := &keyinventory.HSMKeyInfo{Label: entry.label, Purpose: entry.purpose, ActiveVersion: 1}
		if row, ok := versions[entry.label]; ok {
			info.ActiveVersion = row.ActiveVersion
			updatedAt := row.UpdatedAt.UTC().Format(time.RFC3339)
			info.UpdatedAt = &updatedAt
		}
		keys = append(keys, info)
	}
	return &keyinventory.KeyInventoryResponse{Keys: keys}, nil
}
