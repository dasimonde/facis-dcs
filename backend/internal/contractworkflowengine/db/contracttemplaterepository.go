package db

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

// ErrTemplateNotUsable refuses a template that is not registered or published,
// or that has a newer registered version (DCS-FR-TR-17, DCS-FR-TR-19).
var ErrTemplateNotUsable = errors.New("no usable contract template with this DID: it is not registered, or a newer version is registered")

type ContractTemplateQueryResult struct {
	TemplateData    *datatype.JSON `db:"template_data"`
	TemplateVersion int            `db:"version"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	// ComplianceStatus is the result of the template policy audit at approval (DCS-FR-TR-07).
	ComplianceStatus string `db:"compliance_status"`
}

type ContractTemplateMetadata struct {
	DID          string    `db:"did"`
	Version      int       `db:"version"`
	State        string    `db:"state"`
	TemplateType string    `db:"template_type"`
	PartyType    string    `db:"party_type"`
	Name         *string   `db:"name"`
	Description  *string   `db:"description"`
	CreatedBy    string    `db:"created_by"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
	BaseTemplate *string   `db:"base_template"`
}

type ContractTemplateRepo interface {
	ReadContractTemplateDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractTemplateQueryResult, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx) ([]ContractTemplateMetadata, error)
}
