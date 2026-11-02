package db

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"digital-contracting-service/internal/base/datatype"
)

var ErrContractTemplateNotFound = errors.New("template not found")

// ErrInvalidTemplateState refuses a command that the template's current state does not allow.
var ErrInvalidTemplateState = errors.New("invalid contract template state")

type ContractTemplate struct {
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
	// ContentUpdatedAt moves only when the content changes (see ContractTemplateProcessData).
	ContentUpdatedAt time.Time      `db:"content_updated_at"`
	TemplateData     *datatype.JSON `db:"template_data"`
	BaseTemplate     *string        `db:"base_template"`
	SearchableMetadata
	Structure
}

// SearchableMetadata is the template metadata searched by DCS-FR-TR-10.
type SearchableMetadata struct {
	Jurisdiction     *string `db:"jurisdiction"`
	Industry         *string `db:"industry"`
	ComplianceStatus string  `db:"compliance_status"`
}

// Structure is the hierarchy level of the template and the templates it
// requires (DCS-FR-TR-02, DCS-FR-TR-23).
type Structure struct {
	TemplateLevel     string         `db:"template_level"`
	RequiredTemplates pq.StringArray `db:"required_templates"`
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
	Outdated     *bool     `db:"outdated"`
	LatestDID    *string   `db:"latest_did"`
	SearchableMetadata
	Structure
}

type ContractTemplateProcessData struct {
	DID       string    `db:"did"`
	Version   int       `db:"version"`
	State     string    `db:"state"`
	CreatedBy string    `db:"created_by"`
	UpdatedAt time.Time `db:"updated_at"`
	// ContentUpdatedAt moves only when the content (template data, name,
	// description, classification or structure) changes, so the
	// optimistic-lock guard distinguishes a real concurrent content edit from a
	// benign write (a state transition, the background PDF write) that merely
	// nudged updated_at.
	ContentUpdatedAt time.Time `db:"content_updated_at"`
	// CatalogueAssetHash is the Federated Catalogue asset of the published version (DCS-NFR-BR-09).
	CatalogueAssetHash *string `db:"catalogue_asset_hash"`
}

type ContractTemplateUpdateData struct {
	DID          string         `db:"did"`
	State        string         `db:"state"`
	TemplateType string         `db:"template_type"`
	PartyType    string         `db:"party_type"`
	Name         *string        `db:"name"`
	Description  *string        `db:"description"`
	TemplateData *datatype.JSON `db:"template_data"`
	Jurisdiction *string        `db:"jurisdiction"`
	Industry     *string        `db:"industry"`
	// TemplateLevel and RequiredTemplates are left unchanged when nil.
	TemplateLevel     *string
	RequiredTemplates []string
}

type ContractTemplateHistory struct {
	ID           string         `db:"id"`
	DID          string         `db:"did"`
	Version      int            `db:"version"`
	State        string         `db:"state"`
	TemplateType string         `db:"template_type"`
	PartyType    string         `db:"party_type"`
	Name         *string        `db:"name"`
	Description  *string        `db:"description"`
	CreatedBy    string         `db:"created_by"`
	CreatedAt    time.Time      `db:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at"`
	TemplateData *datatype.JSON `db:"template_data"`
	BaseTemplate *string        `db:"base_template"`
	SearchableMetadata
	Structure
}

type SearchValues struct {
	DID          string
	Version      int
	State        string
	TemplateType string
	PartyType    string
	Name         string
	Description  string
	TemplateData string

	Jurisdiction     string
	Industry         string
	ComplianceStatus string

	// CreatedBy matches part of the creator identifier. CreatedFrom and
	// CreatedTo are days (YYYY-MM-DD); both ends are included.
	CreatedBy   string
	CreatedFrom string
	CreatedTo   string
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
	// UpdateState changes the state only when the stored state is from;
	// otherwise it returns ErrInvalidTemplateState.
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, from string, to string) error
	Update(ctx context.Context, tx *sqlx.Tx, data ContractTemplateUpdateData) error
	ReadPDFState(ctx context.Context, tx *sqlx.Tx, did string) (*ContractTemplatePDFState, error)
	// ReadDIDsMissingStoredPDF returns the DIDs of at most limit templates that
	// have no PDF in the artifact store — the work list of the background
	// regenerator's retry pass. Templates in excludeDIDs (attempts the caller
	// has given up on) are left out, so a permanently unrenderable template
	// cannot occupy the batch forever.
	ReadDIDsMissingStoredPDF(ctx context.Context, tx *sqlx.Tx, limit int, excludeDIDs []string) ([]string, error)
	UpdatePDFState(ctx context.Context, tx *sqlx.Tx, did string, data ContractTemplatePDFState) error
	InsertProvenanceCredential(ctx context.Context, tx *sqlx.Tx, data TemplateProvenanceCredential) error
	ReadProvenanceCredentials(ctx context.Context, tx *sqlx.Tx, did string) ([]TemplateProvenanceCredential, error)
	ReadLatestProvenanceVCID(ctx context.Context, tx *sqlx.Tx, did string) (*string, error)
	UpdateComplianceStatus(ctx context.Context, tx *sqlx.Tx, did string, status string) error
	UpdateCatalogueAssetHash(ctx context.Context, tx *sqlx.Tx, did string, hash string) error
	// ReadEvents returns the template's audit events of the given types, oldest first.
	ReadEvents(ctx context.Context, tx *sqlx.Tx, did string, eventTypes ...string) ([]TemplateEvent, error)
	// CountOpenContracts counts the contracts made from the template that are
	// not in a terminal state.
	CountOpenContracts(ctx context.Context, tx *sqlx.Tx, did string) (int, error)
	// ReadPredecessorDID returns the newest older version of the template's
	// lineage that was registered, or nil.
	ReadPredecessorDID(ctx context.Context, tx *sqlx.Tx, did string) (*string, error)
}

// TemplateEvent is one audit event of a template.
type TemplateEvent struct {
	EventType string        `db:"event_type"`
	EventData datatype.JSON `db:"event_data"`
	CreatedAt time.Time     `db:"created_at"`
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
