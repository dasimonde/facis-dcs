package db

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

var ErrContractTemplateNotFound = errors.New("template not found")

type ContractTemplate struct {
	DID            string         `db:"did"`
	DocumentNumber *string        `db:"document_number"`
	Version        int            `db:"version"`
	State          string         `db:"state"`
	TemplateType   string         `db:"template_type"`
	Name           *string        `db:"name"`
	Description    *string        `db:"description"`
	CreatedBy      string         `db:"created_by"`
	CreatedAt      time.Time      `db:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"`
	TemplateData   *datatype.JSON `db:"template_data"`
	BaseTemplate   *string        `db:"base_template"`
}

type ContractTemplateMetadata struct {
	DID            string    `db:"did"`
	DocumentNumber *string   `db:"document_number"`
	Version        int       `db:"version"`
	State          string    `db:"state"`
	TemplateType   string    `db:"template_type"`
	Name           *string   `db:"name"`
	Description    *string   `db:"description"`
	CreatedBy      string    `db:"created_by"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
	BaseTemplate   *string   `db:"base_template"`
	Outdated       *bool     `db:"outdated"`
	LatestDID      *string   `db:"latest_did"`
}

type ContractTemplateProcessData struct {
	DID            string    `db:"did"`
	DocumentNumber *string   `db:"document_number"`
	Version        int       `db:"version"`
	State          string    `db:"state"`
	CreatedBy      string    `db:"created_by"`
	UpdatedAt      time.Time `db:"updated_at"`
	// ContentUpdatedAt moves only when template_data actually changes, so the
	// optimistic-lock guard distinguishes a real concurrent content edit from a
	// benign write (a state transition, the background PDF write) that merely
	// nudged updated_at.
	ContentUpdatedAt time.Time `db:"content_updated_at"`
}

type ContractTemplateUpdateData struct {
	DID            string         `db:"did"`
	DocumentNumber *string        `db:"document_number"`
	State          string         `db:"state"`
	TemplateType   string         `db:"template_type"`
	Name           *string        `db:"name"`
	Description    *string        `db:"description"`
	TemplateData   *datatype.JSON `db:"template_data"`
}

type ContractTemplateHistory struct {
	ID             string         `db:"id"`
	DID            string         `db:"did"`
	DocumentNumber *string        `db:"document_number"`
	Version        int            `db:"version"`
	State          string         `db:"state"`
	TemplateType   string         `db:"template_type"`
	Name           *string        `db:"name"`
	Description    *string        `db:"description"`
	CreatedBy      string         `db:"created_by"`
	CreatedAt      time.Time      `db:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at"`
	TemplateData   *datatype.JSON `db:"template_data"`
	BaseTemplate   *string        `json:"base_template"`
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

type ContractTemplatePDFState struct {
	IPFSCID         string `db:"pdf_ipfs_cid"`
	RendererVersion string `db:"pdf_renderer_version"`
	C2PAState       string `db:"pdf_c2pa_state"`
	PayloadHash     string `db:"pdf_payload_hash"`
}

type ContractTemplateRepo interface {
	CopyFromDID(ctx context.Context, tx *sqlx.Tx, copyDID string, newDID string) (int, error)
	CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string) error
	Create(ctx context.Context, tx *sqlx.Tx, data ContractTemplate) (*time.Time, error)
	ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ContractTemplateHistory, error)
	ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractTemplate, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]ContractTemplateMetadata, error)
	ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues, pagination datatype.Pagination) ([]ContractTemplateMetadata, error)
	ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractTemplateProcessData, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	Update(ctx context.Context, tx *sqlx.Tx, data ContractTemplateUpdateData) error
	ReadPDFState(ctx context.Context, tx *sqlx.Tx, did string) (*ContractTemplatePDFState, error)
	UpdatePDFState(ctx context.Context, tx *sqlx.Tx, did string, data ContractTemplatePDFState) error
	InsertProvenanceCredential(ctx context.Context, tx *sqlx.Tx, data TemplateProvenanceCredential) error
	ReadProvenanceCredentials(ctx context.Context, tx *sqlx.Tx, did string) ([]TemplateProvenanceCredential, error)
	ReadLatestProvenanceVCID(ctx context.Context, tx *sqlx.Tx, did string) (*string, error)
}

// TemplateProvenanceCredential is one registered template version's signed
// W3C provenance VC (DCS-FR-TR-09), linked to its predecessor by vc_id.
type TemplateProvenanceCredential struct {
	DID          string        `db:"did"`
	Version      int           `db:"version"`
	VCID         string        `db:"vc_id"`
	PreviousVCID *string       `db:"previous_vc_id"`
	Credential   datatype.JSON `db:"credential"`
	CreatedAt    time.Time     `db:"created_at"`
}
