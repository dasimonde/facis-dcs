package db

import (
	"context"
	"database/sql/driver"
	"digital-contracting-service/internal/base/datatype"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrContractTemplateNotFound = errors.New("template not found")

type ResponsiblePersons struct {
	Creator   string   `json:"creator"`
	Approver  string   `json:"approver"`
	Reviewers []string `json:"reviewers"`
}

func (r ResponsiblePersons) Value() (driver.Value, error) {
	return json.Marshal(r)
}

func (r *ResponsiblePersons) Scan(src any) error {
	if src == nil {
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("unsupported type: %T", src)
	}
	return json.Unmarshal(b, r)
}

type ContractTemplate struct {
	DID                string              `db:"did"`
	DocumentNumber     *string             `db:"document_number"`
	Version            int                 `db:"version"`
	State              string              `db:"state"`
	TemplateType       string              `db:"template_type"`
	Name               *string             `db:"name"`
	Description        *string             `db:"description"`
	CreatedBy          string              `db:"created_by"`
	CreatedAt          time.Time           `db:"created_at"`
	UpdatedAt          time.Time           `db:"updated_at"`
	ResponsiblePersons *ResponsiblePersons `db:"responsible_persons"`
	TemplateData       *datatype.JSON      `db:"template_data"`
}

type ContractTemplateMetadata struct {
	DID                string              `db:"did"`
	DocumentNumber     *string             `db:"document_number"`
	Version            int                 `db:"version"`
	State              string              `db:"state"`
	TemplateType       string              `db:"template_type"`
	Name               *string             `db:"name"`
	Description        *string             `db:"description"`
	CreatedBy          string              `db:"created_by"`
	CreatedAt          time.Time           `db:"created_at"`
	ResponsiblePersons *ResponsiblePersons `db:"responsible_persons"`
	UpdatedAt          time.Time           `db:"updated_at"`
}

type ContractTemplateProcessData struct {
	DID            string    `db:"did"`
	DocumentNumber *string   `db:"document_number"`
	Version        int       `db:"version"`
	State          string    `db:"state"`
	CreatedBy      string    `db:"created_by"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type ContractTemplateUpdateData struct {
	DID                string              `db:"did"`
	DocumentNumber     *string             `db:"document_number"`
	State              string              `db:"state"`
	TemplateType       string              `db:"template_type"`
	Name               *string             `db:"name"`
	Description        *string             `db:"description"`
	ResponsiblePersons *ResponsiblePersons `db:"responsible_persons"`
	TemplateData       *datatype.JSON      `db:"template_data"`
}

type ContractTemplateHistory struct {
	ID                 string              `db:"id"`
	DID                string              `db:"did"`
	DocumentNumber     *string             `db:"document_number"`
	Version            int                 `db:"version"`
	State              string              `db:"state"`
	TemplateType       string              `db:"template_type"`
	Name               *string             `db:"name"`
	Description        *string             `db:"description"`
	CreatedBy          string              `db:"created_by"`
	CreatedAt          time.Time           `db:"created_at"`
	UpdatedAt          time.Time           `db:"updated_at"`
	ResponsiblePersons *ResponsiblePersons `db:"responsible_persons"`
	TemplateData       *datatype.JSON      `db:"template_data"`
}

type SearchValues struct {
	DID            string
	DocumentNumber string
	Version        int
	State          string
	TemplateType   string
	Name           string
	Description    string
	TemplateData   string
}

type ContractTemplateRepo interface {
	CopyFromDID(ctx context.Context, tx *sqlx.Tx, copyDID string, newDID string) (int, error)
	CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string) error
	Create(ctx context.Context, tx *sqlx.Tx, data ContractTemplate) (*time.Time, error)
	ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ContractTemplateHistory, error)
	ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractTemplate, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx) ([]ContractTemplateMetadata, error)
	ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues) ([]ContractTemplateMetadata, error)
	ReadProcessData(ctx context.Context, tx *sqlx.Tx, did string) (*ContractTemplateProcessData, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	Update(ctx context.Context, tx *sqlx.Tx, data ContractTemplateUpdateData) error
}
