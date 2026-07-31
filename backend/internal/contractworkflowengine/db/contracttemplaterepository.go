package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

type ContractTemplateQueryResult struct {
	TemplateData    *datatype.JSON `db:"template_data"`
	TemplateVersion int            `db:"version"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
}

type ContractTemplateMetadata struct {
	DID          string    `db:"did"`
	Version      int       `db:"version"`
	State        string    `db:"state"`
	TemplateType string    `db:"template_type"`
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
