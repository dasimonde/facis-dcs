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
	// RenewsContractDID is the contract this one renews (dcs:renewsContract).
	RenewsContractDID *string `db:"renews_contract_did"`
	// The termination record of a terminated archived contract
	// (DCS-FR-CSA-16); nil for any other contract.
	TerminationReason *string    `db:"termination_reason"`
	TerminatedAt      *time.Time `db:"terminated_at"`
	TerminationRole   *string    `db:"termination_role"`
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
	// ModifiedBy and ModifiedAt identify the save that replaced this version.
	// They are NULL on rows snapshotted before the actor was recorded.
	ModifiedBy *string    `db:"modified_by"`
	ModifiedAt *time.Time `db:"modified_at"`
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

// ExecutionProof is the proof of contract execution over every signature
// (DCS-FR-SM-10), with the notary and TSA receipts over its content hash.
type ExecutionProof struct {
	DID           string
	ContentHash   string
	Proof         datatype.JSON
	NotaryReceipt *datatype.JSON
	TSAReceipt    *datatype.JSON
}

// ListOptions narrows and orders the contract list before the page is cut
// (DCS-IR-CWE-11).
type ListOptions struct {
	// States keeps contracts in one of these states. Empty keeps all.
	States []string
	// ParentDID keeps contracts whose dcs:parentContract references this DID.
	ParentDID string
	// SortBy is created_at, updated_at, state or name. Empty is created_at.
	SortBy string
	// Ascending sorts from low to high. The default is from high to low.
	Ascending bool
}

type SearchValues struct {
	DID             string
	ContractVersion int
	// States keeps contracts in one of these states. Empty keeps all.
	States       []string
	Name         string
	Description  string
	ContractData string
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
	// TemplateDID matches the template the contract was created from
	// (DCS-IR-CWE-07).
	TemplateDID string
	// Jurisdiction matches the value of the contract field whose shape is the
	// contract-jurisdiction taxonomy concept (DCS-FR-CWE-13, DCS-FR-CSA-10).
	Jurisdiction string
}

// SignedSignature is one SIGNED signature of a contract.
type SignedSignature struct {
	SignerDID      string    `db:"signer_did"`
	CredentialType string    `db:"credential_type"`
	FieldName      string    `db:"field_name"`
	SignedAt       time.Time `db:"signed_at"`
	PDFHash        string    `db:"pdf_hash"`
	JAdESSignature string    `db:"jades_signature"`
}

type ContractPDFState struct {
	IPFSCID         string `db:"pdf_ipfs_cid"`
	RendererVersion string `db:"pdf_renderer_version"`
	C2PAState       string `db:"pdf_c2pa_state"`
	PayloadHash     string `db:"pdf_payload_hash"`
	// Signed is true when the stored PDF carries a PAdES signature. UpdatePDFState
	// never clears it: an incremental update keeps every signature.
	Signed bool `db:"pdf_signed"`
}

type ContractRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data Contract) error
	RemoteCreate(ctx context.Context, tx *sqlx.Tx, data Contract) error
	// CreateHistoryEntryForDID snapshots the current version before a save
	// replaces it, attributed to modifiedBy, the actor of that save.
	CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string, modifiedBy string) error
	ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ContractHistory, error)
	ReadDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*Contract, error)
	ExistsByDID(ctx context.Context, tx *sqlx.Tx, did string) (bool, error)
	// ReadChildrenDIDs returns the DIDs of all locally-known contracts whose
	// dcs:parentContract references did, ordered by did.
	ReadChildrenDIDs(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error)
	// ReadOpenRenewalDID returns a renewal of originalDID that is not
	// withdrawn, revoked, terminated or expired, or "" when there is none.
	ReadOpenRenewalDID(ctx context.Context, tx *sqlx.Tx, originalDID string) (string, error)
	ReadExpiredContracts(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	// ReadRenewalDueContracts returns the SIGNED or ACTIVE contracts whose
	// notice period before exp_date has started and that have no renewal
	// notice for their current exp_date yet (DCS-FR-CWE-22).
	ReadRenewalDueContracts(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	// ClaimRenewalNotice records the renewal notice for (did, expDate). It
	// returns false when the notice is already recorded, so the caller emits
	// the event at most once per expiry date.
	ClaimRenewalNotice(ctx context.Context, tx *sqlx.Tx, did string, expDate time.Time) (bool, error)
	StoreArchiveEntry(ctx context.Context, tx *sqlx.Tx, data ContractArchiveEntry) error
	// AppendArchivePeerSignatures appends to signature_metadata.peer_signatures
	// of the latest active archive entry of did each entry of peerSignatures
	// (a JSON array) that the entry does not hold yet. It returns false when
	// did has no active archive entry or nothing was new.
	AppendArchivePeerSignatures(ctx context.Context, tx *sqlx.Tx, did string, peerSignatures []byte) (bool, error)
	// ReadLatestArchiveEntry returns the latest active archive entry of did,
	// or nil when there is none.
	ReadLatestArchiveEntry(ctx context.Context, tx *sqlx.Tx, did string) (*ContractArchiveEntry, error)
	// StoreExecutionProof keeps a notarized proof of execution of did. It
	// returns false when the same proof is already kept.
	StoreExecutionProof(ctx context.Context, tx *sqlx.Tx, proof ExecutionProof) (bool, error)
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
	// ReadSignedSignatures returns the SIGNED signatures of the contract in the
	// order they were applied.
	ReadSignedSignatures(ctx context.Context, tx *sqlx.Tx, did string) ([]SignedSignature, error)
	ReadArchivedContracts(ctx context.Context, tx *sqlx.Tx) ([]ContractMetadata, error)
	ReadArchivedContractsByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues) ([]ContractMetadata, error)
	ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadProcessDataByDIDOrNil(ctx context.Context, tx *sqlx.Tx, did string) (*ContractProcessData, error)
	ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination, options ListOptions) ([]ContractMetadata, error)
	ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values SearchValues, pagination datatype.Pagination) ([]ContractMetadata, error)
	// ExpireContract persists EXPIRED for did if its expiry has passed and it
	// is in a state that expires (the ReadExpiredContracts condition). It
	// returns false when another writer expired or ended the contract first,
	// so the caller emits ContractExpired at most once.
	ExpireContract(ctx context.Context, tx *sqlx.Tx, did string) (bool, error)
	// UpdateState moves the contract from state from to state to. It fails
	// with contractstate.ErrInvalidTransition when the contract is no longer
	// in state from, so a concurrent writer is never overwritten.
	UpdateState(ctx context.Context, tx *sqlx.Tx, did, from, to string) error
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
