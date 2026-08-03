package db

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

type Responsible struct {
	Creator     string   `json:"creator"`
	Approvers   []string `json:"approvers"`
	Reviewers   []string `json:"reviewers"`
	Negotiators []string `json:"negotiators"`
	// Counterparty is the single peer DCS this contract is offered to and
	// negotiated with (ADR-13). It is NOT a role assignment — reviewer/approver/
	// negotiator are internal RBAC roles held by local users, never peer DIDs.
	// Origin + Counterparty are the two parties (GetParties): the PDF ship
	// targets and the signature-field slots.
	Counterparty string `json:"counterparty"`
}

func (r *Responsible) Value() (driver.Value, error) {
	return json.Marshal(r)
}

func (r *Responsible) Scan(src any) error {
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

// GetParties returns the contract's two DCS parties — the origin (creator) and
// the counterparty — deduplicated, empty entries dropped. These are the PDF
// ship targets and the slots the AcroForm signature fields are seeded for
// (ADR-13); they are distinct from the internal RBAC role lists.
func (r *Responsible) GetParties() []string {
	parties := make([]string, 0, 2)
	for _, did := range []string{r.Creator, r.Counterparty} {
		if did != "" && !slices.Contains(parties, did) {
			parties = append(parties, did)
		}
	}
	return parties
}

type Contract struct {
	DID             string         `db:"did"`
	Origin          string         `db:"origin"`
	ContractVersion int            `db:"contract_version"`
	State           string         `db:"state"`
	CreatedBy       string         `db:"created_by"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
	StartDate       *time.Time     `db:"start_date"`
	ExpDate         *time.Time     `db:"exp_date"`
	ExpPolicy       *string        `db:"exp_policy"`
	ExpNoticePeriod *int           `db:"exp_notice_period"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	Responsible     *Responsible   `db:"responsible"`
	ContractData    *datatype.JSON `db:"contract_data"`
	TemplateDID     string         `db:"template_did"`
	TemplateVersion int            `db:"template_version"`
	// TargetID is the registered Contract Target System this contract deploys
	// to (ADR-25); nil until one is designated.
	TargetID *string `db:"target_id"`
}

type ContractMetadata struct {
	DID                  string       `db:"did"`
	Origin               string       `db:"origin"`
	ContractVersion      int          `db:"contract_version"`
	State                string       `db:"state"`
	CreatedBy            string       `db:"created_by"`
	CreatedAt            time.Time    `db:"created_at"`
	UpdatedAt            time.Time    `db:"updated_at"`
	StartDate            *time.Time   `db:"start_date"`
	ExpDate              *time.Time   `db:"exp_date"`
	ExpPolicy            *string      `db:"exp_policy"`
	ExpNoticePeriod      *int         `db:"exp_notice_period"`
	Name                 *string      `db:"name"`
	Responsible          *Responsible `db:"responsible"`
	Description          *string      `db:"description"`
	TemplateDID          string       `db:"template_did"`
	TemplateVersion      int          `db:"template_version"`
	Outdated             *bool        `db:"outdated"`
	LatestTemplateDID    *string      `db:"latest_template_did"`
	TemplateIsDeprecated *bool        `db:"template_is_deprecated"`
	ParentContractDID    *string      `db:"parent_contract_did"`
	// The registered target system this contract deploys to (ADR-25), resolved
	// to its name so the destination is readable without a second lookup.
	TargetID   *string `db:"target_id"`
	TargetName *string `db:"target_name"`
	// Evidence is only populated by the archived-contracts queries (joined
	// from contract_archive_entries.evidence); it is nil for the
	// non-archive metadata queries that share this struct.
	Evidence *datatype.JSON `db:"evidence"`
	// ArchiveSummary/ArchiveTags carry the archive entry's annotation
	// (DCS-FR-CSA-11); like Evidence they are only populated by the
	// archived-contracts queries.
	ArchiveSummary *string        `db:"archive_summary"`
	ArchiveTags    *datatype.JSON `db:"archive_tags"`
}

type ContractProcessData struct {
	DID             string    `db:"did"`
	Origin          string    `db:"origin"`
	ContractVersion int       `db:"contract_version"`
	State           string    `db:"state"`
	CreatedBy       string    `db:"created_by"`
	UpdatedAt       time.Time `db:"updated_at"`
	// ContentUpdatedAt moves only when contract_data actually changes, so the
	// optimistic-lock guard distinguishes a real concurrent content edit from a
	// benign write that merely nudged updated_at.
	ContentUpdatedAt time.Time  `db:"content_updated_at"`
	StartDate        *time.Time `db:"start_date"`
	ExpDate          *time.Time `db:"exp_date"`
	ExpPolicy        *string    `db:"exp_policy"`
	ExpNoticePeriod  *int       `db:"exp_notice_period"`
}

type ContractUpdateData struct {
	DID             string         `db:"did"`
	State           string         `db:"state"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	ContractVersion int            `db:"contract_version"`
	ContractData    *datatype.JSON `db:"contract_data"`
	StartDate       *time.Time     `db:"start_date"`
	ExpDate         *time.Time     `db:"exp_date"`
	ExpPolicy       *string        `db:"exp_policy"`
	ExpNoticePeriod *int           `db:"exp_notice_period"`
	Responsible     *Responsible   `db:"responsible"`
}

type ContractHistory struct {
	ID              string         `db:"id"`
	Origin          string         `db:"origin"`
	DID             string         `db:"did"`
	ContractVersion int            `db:"contract_version"`
	State           string         `db:"state"`
	CreatedBy       string         `db:"created_by"`
	CreatedAt       time.Time      `db:"created_at"`
	UpdatedAt       time.Time      `db:"updated_at"`
	StartDate       *time.Time     `db:"start_date"`
	ExpDate         *time.Time     `db:"exp_date"`
	ExpPolicy       *string        `db:"exp_policy"`
	ExpNoticePeriod *int           `db:"exp_notice_period"`
	Name            *string        `db:"name"`
	Description     *string        `db:"description"`
	Responsible     *Responsible   `db:"responsible"`
	ContractData    *datatype.JSON `db:"contract_data"`
	TemplateDID     string         `db:"template_did"`
	TemplateVersion int            `db:"template_version"`
}

type ContractArchiveEntry struct {
	DID                  string         `db:"did"`
	ContractVersion      int            `db:"contract_version"`
	StoredBy             string         `db:"stored_by"`
	StoredAt             time.Time      `db:"stored_at"`
	ArchiveStatus        string         `db:"archive_status"`
	ContractSnapshot     datatype.JSON  `db:"contract_snapshot"`
	ContentHash          string         `db:"content_hash"`
	SnapshotCID          string         `db:"snapshot_cid"`
	SnapshotCIDCreatedAt time.Time      `db:"snapshot_cid_created_at"`
	SignatureMeta        *datatype.JSON `db:"signature_metadata"`
	CredentialHashes     *datatype.JSON `db:"credential_hashes"`
	TSAReceipt           *datatype.JSON `db:"tsa_receipt"`
	Evidence             *datatype.JSON `db:"evidence"`
	RetentionUntil       *time.Time     `db:"retention_until"`
	DeletedAt            *time.Time     `db:"deleted_at"`
	DeletedBy            *string        `db:"deleted_by"`
	DeletionReason       *string        `db:"deletion_reason"`
}

type SearchValues struct {
	DID             string
	ContractVersion int
	State           string
	Name            string
	Description     string
	ContractData    string
	// Tag filters archived contracts by an assigned annotation tag
	// (DCS-FR-CSA-11); only meaningful for the archive queries, whose
	// backing view exposes archive_tags.
	Tag string
	// Party matches a contract party DID against the responsible JSONB
	// column: the creator or the counterparty (DCS-FR-CSA-10, DCS-FR-CSA-13).
	Party string
	// ValidFrom/ValidUntil bound the contract's validity period
	// (DCS-FR-CSA-10, DCS-FR-CSA-13): start_date >= ValidFrom,
	// exp_date <= ValidUntil. Each bound applies independently when set.
	ValidFrom  *time.Time
	ValidUntil *time.Time
	// ParentDID is the full-scope hierarchy filter: when
	// set, only contracts whose dcs:parentContract references this DID are
	// returned. It is a reverse-index QUERY over children the instance
	// legitimately holds locally — never a field on the parent document.
	ParentDID string
}

type ContractPDFState struct {
	IPFSCID         string `db:"pdf_ipfs_cid"`
	RendererVersion string `db:"pdf_renderer_version"`
	C2PAState       string `db:"pdf_c2pa_state"`
	PayloadHash     string `db:"pdf_payload_hash"`
}

type ContractRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data Contract) error
	RemoteCreate(ctx context.Context, tx *sqlx.Tx, data Contract) error
	CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string) error
	ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ContractHistory, error)
	ReadDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*Contract, error)
	ExistsByDID(ctx context.Context, tx *sqlx.Tx, did string) (bool, error)
	// ReadChildrenDIDs returns the DIDs of all locally-known contracts whose
	// dcs:parentContract references did, ordered by did.
	ReadChildrenDIDs(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error)
	ReadExpiredContracts(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	StoreArchiveEntry(ctx context.Context, tx *sqlx.Tx, data ContractArchiveEntry) error
	ReadArchiveEntries(ctx context.Context, tx *sqlx.Tx) ([]ContractArchiveEntry, error)
	// MarkArchiveEntryDeleted soft-deletes every not-yet-deleted archive
	// entry for did (DCS-FR-CSA-17): sets deleted_at/deleted_by/
	// deletion_reason rather than removing the row, so the evidence stays
	// discoverable for compliance/dispute resolution. Returns the number of
	// entries marked (0 if did has no archive entries, or all its entries
	// were already deleted).
	MarkArchiveEntryDeleted(ctx context.Context, tx *sqlx.Tx, did string, deletedBy string, reason string) (int, error)
	// AnnotateArchiveEntry sets the summary and (when tags is non-nil,
	// replacing the whole set) the tags of every not-deleted archive entry
	// for did (DCS-FR-CSA-11). Only the annotation columns are touched —
	// the entry's snapshot/evidence stay immutable. Returns the number of
	// entries annotated (0 if did has no live archive entries).
	AnnotateArchiveEntry(ctx context.Context, tx *sqlx.Tx, did string, summary string, tags *datatype.JSON) (int, error)
	// ReadSignedSignatureFieldNames returns the field names of all SIGNED
	// (non-revoked) signatures on the contract — the deploy gate compares
	// them against the contract document's declared signatureFields
	// (DCS-FR-SM-07/-17, DCS-NFR-BR-03).
	ReadSignedSignatureFieldNames(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error)
	ReadArchivedContracts(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	ReadArchivedContractsByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues) ([]ContractMetadata, error)
	ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadProcessDataByDIDOrNil(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]ContractMetadata, error)
	ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues, pagination datatype.Pagination) ([]ContractMetadata, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	Update(ctx context.Context, tx *sqlx.Tx, data ContractUpdateData) error
	RemoteUpdate(ctx context.Context, tx *sqlx.Tx, data Contract) error
	ReadPDFState(ctx context.Context, tx *sqlx.Tx, did string) (*ContractPDFState, error)
	// CountSignedSignatures returns how many committed (SIGNED) signatures the
	// contract carries. A contract that carries one holds a PAdES-signed PDF
	// that must never be re-rendered: a fresh render replaces the signed bytes
	// outright. Appending to it is a different operation and is allowed —
	// further signatures and the ADR-26 provenance re-anchor are both
	// incremental updates (DCS-OR-C2PA-002, DCS-OR-C2PA-010).
	CountSignedSignatures(ctx context.Context, tx *sqlx.Tx, did string) (int, error)
	// ReadDIDsNeedingRegeneration returns the DIDs of at most limit contracts
	// whose stored PDF is not a render of the document as it now stands, oldest
	// first — the work list of the background regenerator's retry pass. Two
	// things put a contract on it: no PDF in the artifact store at all, and a
	// PDF whose recorded payload hash no longer matches the stored
	// contract_data. The second is what a regeneration LOST to a transient
	// pdf-core or artifact-store failure leaves behind, and nothing else
	// re-renders it: the lifecycle event that asked for it is not redelivered,
	// and the federation ship refuses such a PDF outright
	// (dcstodcs.holdsSupersededPDF), so without this the contract would never be
	// shippable again.
	//
	// Signed contracts are left out: the only reason one appears is that its
	// artifact is MISSING, and the regenerator's answer to a missing artifact is
	// a fresh render — an unsigned PDF stamped as authoritative over the peer's
	// provenance. So is any artifact past "draft", whose recorded hash comes
	// from signing rather than from a render and so is not expected to match.
	// Contracts in excludeDIDs (attempts the caller has given up on) are left
	// out too, so a permanently stuck row cannot occupy the batch and starve the
	// transient failures behind it.
	ReadDIDsNeedingRegeneration(ctx context.Context, tx *sqlx.Tx, limit int, excludeDIDs []string) ([]string, error)
	UpdatePDFState(ctx context.Context, tx *sqlx.Tx, did string, data ContractPDFState) error
}
