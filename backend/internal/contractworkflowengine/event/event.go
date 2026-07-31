// Package event defines the contract workflow engine's domain events — one
// struct per Goa endpoint/state transition, each implementing base/event.Event
// (EventType/GetDID). Handlers pass these to base/event.Create in the same
// DB transaction as their mutation; base/event.OutboxProcessor later anchors
// them to the tamper-evident audit trail and republishes them on NATS, where
// this package's own remote-sync events (RemoteActionRequestEvent,
// RemoteSyncEvent, RemoteSyncRequestEvent, OutdatedPeerEvent) are also
// consumed by dcstodcs to drive/guard cross-peer synchronization.
package event

import (
	"time"

	"digital-contracting-service/internal/contractworkflowengine/db"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/actionflag"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
)

// RemoteActionRequestEvent is emitted when a action on main peer ist triggered
type RemoteActionRequestEvent struct {
	DID         string    `json:"did"`
	OccurredAt  time.Time `json:"occurred_at"`
	Action      string    `json:"action"`
	FromPeerDID string    `json:"from_peer_did"`
	MainPeerDID string    `json:"main_peer_did"`
	Component   string    `json:"component"`
}

// EventType implements the Event interface.
func (e RemoteActionRequestEvent) EventType() string {
	return eventtype.RemoteActionRequestEvent.String()
}

// GetDID implements the Event interface.
func (e RemoteActionRequestEvent) GetDID() string {
	return e.DID
}

// PdfRegeneratedEvent is emitted after the background regenerator stored a
// contract's PDF (ADR-13). The DCS-to-DCS synchronizer consumes it to ship the
// PDF to the counterparty on shippable transitions.
type PdfRegeneratedEvent struct {
	DID        string    `json:"did"`
	IPFSCID    string    `json:"ipfs_cid"`
	State      string    `json:"state"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e PdfRegeneratedEvent) EventType() string {
	return eventtype.PDFRegenerated.String()
}

// GetDID implements the Event interface.
func (e PdfRegeneratedEvent) GetDID() string {
	return e.DID
}

// CreateEvent is emitted when a new contract is created.
type CreateEvent struct {
	DID          string             `json:"did"`
	HolderDID    string             `json:"holder_did"`
	TemplateDID  string             `json:"template_did"`
	CreatedBy    string             `json:"created_by"`
	Name         *string            `json:"name"`
	Description  *string            `json:"description"`
	ContractData *datatype.JSON     `json:"contract_data"`
	OccurredAt   time.Time          `json:"occurred_at"`
	UserRoles    userrole.UserRoles `json:"user_roles"`
	Responsible  *db.Responsible    `json:"responsible,omitempty"`
}

// EventType implements the Event interface.
func (e CreateEvent) EventType() string {
	return eventtype.Create.String()
}

// GetDID implements the Event interface.
func (e CreateEvent) GetDID() string {
	return e.DID
}

// RemoteSyncEvent is emitted when a new contract is created.
type RemoteSyncEvent struct {
	DID             string                             `json:"did"`
	TemplateDID     string                             `json:"template_did"`
	CreatedBy       string                             `json:"created_by"`
	Name            *string                            `json:"name"`
	Description     *string                            `json:"description"`
	ContractData    *datatype.JSON                     `json:"contract_data"`
	OccurredAt      time.Time                          `json:"occurred_at"`
	Responsible     *db.Responsible                    `json:"responsible"`
	ContractVersion int                                `json:"contract_version"`
	State           contractstate.ContractState        `json:"state"`
	CreatedAt       time.Time                          `json:"created_at"`
	UpdatedAt       time.Time                          `json:"updated_at"`
	TemplateVersion int                                `json:"template_version"`
	ExpPolicy       *expirationpolicy.ExpirationPolicy `json:"exp_policy"`
	ExpDate         *time.Time                         `json:"exp_date"`
	ExpNoticePeriod *expirationpolicy.ExpirationPolicy `json:"exp_notice_period"`
	StartDate       *time.Time                         `json:"start_date"`
	Origin          string                             `json:"origin"`
	FromPeerDID     string                             `json:"from_peer_did"`
	LocalPeerDID    string                             `json:"local_peer_did"`
}

// EventType implements the Event interface.
func (e RemoteSyncEvent) EventType() string {
	return eventtype.RemoteSync.String()
}

// GetDID implements the Event interface.
func (e RemoteSyncEvent) GetDID() string {
	return e.DID
}

// RemoteSyncRequestEvent is emitted when a new contract is created.
type RemoteSyncRequestEvent struct {
	DID             string                             `json:"did"`
	TemplateDID     string                             `json:"template_did"`
	CreatedBy       string                             `json:"created_by"`
	Name            *string                            `json:"name"`
	Description     *string                            `json:"description"`
	ContractData    *datatype.JSON                     `json:"contract_data"`
	OccurredAt      time.Time                          `json:"occurred_at"`
	Responsible     *db.Responsible                    `json:"responsible"`
	ContractVersion int                                `json:"contract_version"`
	State           contractstate.ContractState        `json:"state"`
	CreatedAt       time.Time                          `json:"created_at"`
	UpdatedAt       time.Time                          `json:"updated_at"`
	TemplateVersion int                                `json:"template_version"`
	ExpPolicy       *expirationpolicy.ExpirationPolicy `json:"exp_policy"`
	ExpDate         *time.Time                         `json:"exp_date"`
	ExpNoticePeriod *expirationpolicy.ExpirationPolicy `json:"exp_notice_period"`
	StartDate       *time.Time                         `json:"start_date"`
	Origin          string                             `json:"origin"`
	FromPeerDID     string                             `json:"from_peer_did"`
	LocalPeerDID    string                             `json:"local_peer_did"`
}

// EventType implements the Event interface.
func (e RemoteSyncRequestEvent) EventType() string {
	return eventtype.RemoteSyncRequest.String()
}

// GetDID implements the Event interface.
func (e RemoteSyncRequestEvent) GetDID() string {
	return e.DID
}

// RecoverOutdatedPeerEvent is emitted when sync fails are handled
type RecoverOutdatedPeerEvent struct {
	DID        string    `json:"did"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e RecoverOutdatedPeerEvent) EventType() string {
	return eventtype.OutdatedPeer.String()
}

// GetDID implements the Event interface.
func (e RecoverOutdatedPeerEvent) GetDID() string {
	return e.DID
}

// OutdatedPeerEvent is emitted when remote contract data is outdated
type OutdatedPeerEvent struct {
	DID             string    `json:"did"`
	OutdatedPeerDID string    `json:"outdated_peer_did"`
	OccurredAt      time.Time `json:"occurred_at"`
	Origin          string    `json:"origin"`
}

// EventType implements the Event interface.
func (e OutdatedPeerEvent) EventType() string {
	return eventtype.OutdatedPeer.String()
}

// GetDID implements the Event interface.
func (e OutdatedPeerEvent) GetDID() string {
	return e.DID
}

// UpdateEvent is emitted when contract data is updated.
type UpdateEvent struct {
	DID                string                             `json:"did"`
	HolderDID          string                             `json:"holder_did"`
	UpdatedBy          string                             `json:"updated_by"`
	OldName            *string                            `json:"old_name,omitempty"`
	NewName            *string                            `json:"new_name,omitempty"`
	OldDescription     *string                            `json:"old_description,omitempty"`
	NewDescription     *string                            `json:"new_description,omitempty"`
	OldContractData    *datatype.JSON                     `json:"old_contract_data,omitempty"`
	NewContractData    *datatype.JSON                     `json:"new_contract_data,omitempty"`
	OccurredAt         time.Time                          `json:"occurred_at"`
	OldExpDate         *time.Time                         `json:"old_exp_date,omitempty"`
	NewExpDate         *time.Time                         `json:"new_exp_date,omitempty"`
	OldExpPolicy       *expirationpolicy.ExpirationPolicy `json:"old_exp_policy,omitempty"`
	NewExpPolicy       *expirationpolicy.ExpirationPolicy `json:"new_exp_policy,omitempty"`
	OldExpNoticePeriod *int                               `json:"old_exp_notice_period,omitempty"`
	NewExpNoticePeriod *int                               `json:"new_exp_notice_period,omitempty"`
	OldStartDate       *time.Time                         `json:"old_start_date,omitempty"`
	NewStartDate       *time.Time                         `json:"new_start_date,omitempty"`
	UserRoles          userrole.UserRoles                 `json:"user_roles"`
}

// EventType implements the Event interface.
func (e UpdateEvent) EventType() string {
	return eventtype.Update.String()
}

// GetDID implements the Event interface.
func (e UpdateEvent) GetDID() string {
	return e.DID
}

// SubmitEvent is emitted when a contract is submitted
type SubmitEvent struct {
	DID             string                 `json:"did"`
	HolderDID       string                 `json:"holder_did"`
	PreviousState   string                 `json:"previous_state"`
	NewState        string                 `json:"new_state"`
	SubmittedBy     string                 `json:"submitted_by"`
	OccurredAt      time.Time              `json:"occurred_at"`
	ContractVersion int                    `json:"contract_version"`
	ActionFlag      *actionflag.ActionFlag `json:"action_flag,omitempty"`
	Comments        []string               `json:"comments"`
	UserRoles       userrole.UserRoles     `json:"user_roles"`
}

// EventType implements the Event interface.
func (e SubmitEvent) EventType() string {
	return eventtype.Submit.String()
}

// GetDID implements the Event interface.
func (e SubmitEvent) GetDID() string {
	return e.DID
}

// OfferEvent is emitted when a draft contract is offered to the counterparty.
type OfferEvent struct {
	DID             string             `json:"did"`
	HolderDID       string             `json:"holder_did"`
	ContractVersion int                `json:"contract_version"`
	OfferedBy       string             `json:"offered_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e OfferEvent) EventType() string {
	return eventtype.Offer.String()
}

// GetDID implements the Event interface.
func (e OfferEvent) GetDID() string {
	return e.DID
}

// WithdrawEvent is emitted when the initiator withdraws a contract before
// approval.
type WithdrawEvent struct {
	DID             string             `json:"did"`
	HolderDID       string             `json:"holder_did"`
	ContractVersion int                `json:"contract_version"`
	WithdrawnBy     string             `json:"withdrawn_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e WithdrawEvent) EventType() string {
	return eventtype.Withdraw.String()
}

// GetDID implements the Event interface.
func (e WithdrawEvent) GetDID() string {
	return e.DID
}

// RetrieveByIDEvent is emitted when contract data is retrieved.
type RetrieveByIDEvent struct {
	DID         string             `json:"did"`
	HolderDID   string             `json:"holder_did"`
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e RetrieveByIDEvent) EventType() string {
	return eventtype.RetrieveByID.String()
}

// GetDID implements the Event interface.
func (e RetrieveByIDEvent) GetDID() string {
	return e.DID
}

// RetrieveByIDDeniedEvent is emitted when a retrieve_by_id call is refused
// because the caller is not an authorized party of the contract (party
// read-scoping in query/contract/querybyid.go) — the denial itself is part
// of the auditable access history.
type RetrieveByIDDeniedEvent struct {
	DID         string             `json:"did"`
	HolderDID   string             `json:"holder_did"`
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e RetrieveByIDDeniedEvent) EventType() string {
	return eventtype.AccessDenied.String()
}

// GetDID implements the Event interface.
func (e RetrieveByIDDeniedEvent) GetDID() string {
	return e.DID
}

// RetrieveHistoryByDIDEvent is emitted when contract data is retrieved.
type RetrieveHistoryByDIDEvent struct {
	DID         string             `json:"did"`
	HolderDID   string             `json:"holder_did"`
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e RetrieveHistoryByDIDEvent) EventType() string {
	return eventtype.RetrieveHistoryByDID.String()
}

// GetDID implements the Event interface.
func (e RetrieveHistoryByDIDEvent) GetDID() string {
	return e.DID
}

// RetrieveAllEvent is emitted when contract data is retrieved.
type RetrieveAllEvent struct {
	HolderDID   string             `json:"holder_did"`
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// RetrieveArchivedEvent is emitted when archive data is retrieved.
type RetrieveArchivedEvent struct {
	RetrievedBy string    `json:"retrieved_by"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// StoreArchivedEvent is emitted when a contract is stored in the archive.
type StoreArchivedEvent struct {
	DID             string                 `json:"did"`
	ContractVersion int                    `json:"contract_version"`
	StoredBy        string                 `json:"stored_by"`
	ContentHash     string                 `json:"content_hash"`
	SnapshotCID     string                 `json:"snapshot_cid"`
	ArchiveStatus   string                 `json:"archive_status"`
	NotaryReceipt   *ArchiveNotaryReceipt  `json:"notary_receipt,omitempty"`
	TSAReceipt      *ArchiveTSAReceipt     `json:"tsa_receipt,omitempty"`
	EvidenceSummary ArchiveEvidenceSummary `json:"evidence_summary"`
	OccurredAt      time.Time              `json:"occurred_at"`
}

type ArchiveEvidenceSummary struct {
	SnapshotHashAlgorithm string `json:"snapshot_hash_algorithm"`
	SignatureStatus       string `json:"signature_status"`
	CredentialHashStatus  string `json:"credential_hash_status"`
}

type ArchiveNotaryReceipt struct {
	ReceiptType    string    `json:"receiptType"`
	ArchiveEntryID string    `json:"archiveEntryId"`
	EventHash      string    `json:"eventHash"`
	PreviousHash   *string   `json:"previousHash"`
	ReceivedAt     time.Time `json:"receivedAt"`
}

type ArchiveTSAReceipt struct {
	ReceiptType    string    `json:"receipt_type"`
	Token          string    `json:"token"`
	TokenEncoding  string    `json:"token_encoding"`
	HashAlgorithm  string    `json:"hash_algorithm"`
	MessageImprint string    `json:"message_imprint"`
	GeneratedAt    time.Time `json:"generated_at"`
	Policy         string    `json:"policy,omitempty"`
	SerialNumber   string    `json:"serial_number,omitempty"`
}

// EventType implements [event.Event].
func (r RetrieveArchivedEvent) EventType() string {
	return eventtype.RetrieveArchived.String()
}

// GetDID implements [event.Event].
func (r RetrieveArchivedEvent) GetDID() string {
	return "*"
}

// EventType implements [event.Event].
func (e StoreArchivedEvent) EventType() string {
	return eventtype.StoreArchived.String()
}

// GetDID implements [event.Event].
func (e StoreArchivedEvent) GetDID() string {
	return e.DID
}

// DeleteArchivedEvent is emitted when an archive entry is soft-deleted
// (DCS-FR-CSA-17: deletion requires a justification and MUST be logged with
// timestamp and user identity).
type DeleteArchivedEvent struct {
	DID           string    `json:"did"`
	DeletedBy     string    `json:"deleted_by"`
	Justification string    `json:"justification"`
	EntriesMarked int       `json:"entries_marked"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// EventType implements [event.Event].
func (e DeleteArchivedEvent) EventType() string {
	return eventtype.DeleteArchived.String()
}

// GetDID implements [event.Event].
func (e DeleteArchivedEvent) GetDID() string {
	return e.DID
}

// KeyShreddedEvent records the destruction of a contract's wrapped
// content-encryption keys (DCS-NFR-SEC-13, DCS-NFR-COMP-03). ShreddedBy is a
// local participant on the archive-deletion path or the requesting peer's DID
// on the federation erase path. The event carries no contract content.
type KeyShreddedEvent struct {
	DID          string    `json:"did"`
	ScopeKind    string    `json:"scope_kind"`
	ScopeID      string    `json:"scope_id"`
	ShreddedBy   string    `json:"shredded_by"`
	Reason       string    `json:"reason"`
	RowsShredded int64     `json:"rows_shredded"`
	OccurredAt   time.Time `json:"occurred_at"`
}

// EventType implements [event.Event].
func (e KeyShreddedEvent) EventType() string {
	return eventtype.KeyShredded.String()
}

// GetDID implements [event.Event].
func (e KeyShreddedEvent) GetDID() string {
	return e.DID
}

// AnnotateArchivedEvent is emitted when an archive entry's summary/tags
// annotation is set (DCS-FR-CSA-11).
type AnnotateArchivedEvent struct {
	DID         string    `json:"did"`
	AnnotatedBy string    `json:"annotated_by"`
	Summary     string    `json:"summary"`
	Tags        []string  `json:"tags"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// EventType implements [event.Event].
func (e AnnotateArchivedEvent) EventType() string {
	return eventtype.AnnotateArchived.String()
}

// GetDID implements [event.Event].
func (e AnnotateArchivedEvent) GetDID() string {
	return e.DID
}

// EventType implements the Event interface.
func (e RetrieveAllEvent) EventType() string {
	return eventtype.RetrieveAll.String()
}

// GetDID implements the Event interface.
func (e RetrieveAllEvent) GetDID() string {
	return "*"
}

// VerifyEvent is emitted when a template is verified.
type VerifyEvent struct {
	DID             string             `json:"did"`
	HolderDID       string             `json:"holder_did"`
	ContractVersion int                `json:"contract_version"`
	VerifiedBy      string             `json:"verified_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e VerifyEvent) EventType() string {
	return eventtype.Verify.String()
}

// GetDID implements the Event interface.
func (e VerifyEvent) GetDID() string {
	return e.DID
}

// NegotiationEvent is emitted when a template is verified.
type NegotiationEvent struct {
	DID             string             `json:"did"`
	HolderDID       string             `json:"holder_did"`
	ContractVersion int                `json:"contract_version"`
	ChangeRequest   *datatype.JSON     `json:"change_request,omitempty"`
	NegotiatedBy    string             `json:"negotiated_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	Negotiators     []string           `json:"negotiators"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e NegotiationEvent) EventType() string {
	return eventtype.Negotiation.String()
}

// GetDID implements the Event interface.
func (e NegotiationEvent) GetDID() string {
	return e.DID
}

// AcceptNegotiationEvent is emitted when a template is verified.
type AcceptNegotiationEvent struct {
	DID             string             `json:"did"`
	HolderDID       string             `json:"holder_did"`
	ContractVersion int                `json:"contract_version"`
	AcceptedBy      string             `json:"accepted_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e AcceptNegotiationEvent) EventType() string {
	return eventtype.AcceptRespond.String()
}

// GetDID implements the Event interface.
func (e AcceptNegotiationEvent) GetDID() string {
	return e.DID
}

// RejectNegotiationEvent is emitted when a template is verified.
type RejectNegotiationEvent struct {
	DID             string             `json:"did"`
	HolderDID       string             `json:"holder_did"`
	ContractVersion int                `json:"contract_version"`
	RejectedBy      string             `json:"rejected_by"`
	RejectionReason *string            `json:"rejection_reason,omitempty"`
	OccurredAt      time.Time          `json:"occurred_at"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e RejectNegotiationEvent) EventType() string {
	return eventtype.RejectRespond.String()
}

// GetDID implements the Event interface.
func (e RejectNegotiationEvent) GetDID() string {
	return e.DID
}

// ApproveEvent is emitted when a contract is approved.
type ApproveEvent struct {
	DID             string             `json:"did"`
	HolderDID       string             `json:"holder_did"`
	ContractVersion int                `json:"contract_version"`
	ApprovedBy      string             `json:"approved_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ApproveEvent) EventType() string {
	return eventtype.Approve.String()
}

// GetDID implements the Event interface.
func (e ApproveEvent) GetDID() string {
	return e.DID
}

// RejectEvent is emitted when a contract is rejected.
type RejectEvent struct {
	DID             string             `json:"did"`
	HolderDID       string             `json:"holder_did"`
	ContractVersion int                `json:"contract_version"`
	RejectedBy      string             `json:"rejected_by"`
	Reason          string             `json:"reason"`
	OccurredAt      time.Time          `json:"occurred_at"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e RejectEvent) EventType() string {
	return eventtype.Reject.String()
}

// GetDID implements the Event interface.
func (e RejectEvent) GetDID() string {
	return e.DID
}

// TerminateEvent is emitted when a contract is terminated.
type TerminateEvent struct {
	DID             string             `json:"did"`
	HolderDID       string             `json:"holder_did"`
	ContractVersion int                `json:"contract_version"`
	Reason          string             `json:"reason"`
	TerminatedBy    string             `json:"terminated_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e TerminateEvent) EventType() string {
	return eventtype.Terminate.String()
}

// GetDID implements the Event interface.
func (e TerminateEvent) GetDID() string {
	return e.DID
}

// RenewEvent is emitted on the newly created renewal contract when it is
// derived from an existing (original) contract (DCS-FR-CWE-11/22,
// DCS-FR-CSA-15). The original contract is not mutated and does not receive
// a matching event; the link is one-directional (new -> original), recorded
// both here and in the new contract's dcs:renewsContract JSON-LD reference.
type RenewEvent struct {
	DID                     string             `json:"did"`
	HolderDID               string             `json:"holder_did"`
	RenewedBy               string             `json:"renewed_by"`
	OriginalDID             string             `json:"original_did"`
	OriginalContractVersion int                `json:"original_contract_version"`
	ContractData            *datatype.JSON     `json:"contract_data"`
	OccurredAt              time.Time          `json:"occurred_at"`
	UserRoles               userrole.UserRoles `json:"user_roles"`
	Responsible             *db.Responsible    `json:"responsible,omitempty"`
}

// EventType implements the Event interface.
func (e RenewEvent) EventType() string {
	return eventtype.Renew.String()
}

// GetDID implements the Event interface.
func (e RenewEvent) GetDID() string {
	return e.DID
}

// RecordEvidenceEvent is emitted when an evidence is recorded
type RecordEvidenceEvent struct {
	DID             string             `json:"did"`
	HolderDID       string             `json:"holder_did"`
	ContractVersion int                `json:"contract_version"`
	RecordedBy      string             `json:"recorded_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e RecordEvidenceEvent) EventType() string {
	return eventtype.RecordEvidence.String()
}

// GetDID implements the Event interface.
func (e RecordEvidenceEvent) GetDID() string {
	return e.DID
}

// AuditEvent is emitted when the contract is audited
type AuditEvent struct {
	DID           string                      `json:"did"`
	HolderDID     string                      `json:"holder_did"`
	AuditedBy     string                      `json:"audited_by"`
	OccurredAt    time.Time                   `json:"occurred_at"`
	ComponentType componenttype.ComponentType `json:"component_type"`
	UserRoles     userrole.UserRoles          `json:"user_roles"`
}

// EventType implements the Event interface.
func (e AuditEvent) EventType() string {
	return eventtype.Audit.String()
}

// GetDID implements the Event interface.
func (e AuditEvent) GetDID() string {
	return e.DID
}

// ExportEvent is emitted when a contract bundle (ZIP) is exported.
// FR-CSA-18: an export is a retrieval-class action and is recorded in the
// contract's audit trail.
type ExportEvent struct {
	DID        string             `json:"did"`
	HolderDID  string             `json:"holder_did"`
	ExportedBy string             `json:"exported_by"`
	Format     string             `json:"format"`
	OccurredAt time.Time          `json:"occurred_at"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ExportEvent) EventType() string {
	return eventtype.Export.String()
}

// GetDID implements the Event interface.
func (e ExportEvent) GetDID() string {
	return e.DID
}

// ReviewEvent is emitted when contract is reviewed.
type ReviewEvent struct {
	DID        string             `json:"did"`
	HolderDID  string             `json:"holder_did"`
	ReviewedBy string             `json:"reviewed_by"`
	OccurredAt time.Time          `json:"occurred_at"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ReviewEvent) EventType() string {
	return eventtype.Review.String()
}

// GetDID implements the Event interface.
func (e ReviewEvent) GetDID() string {
	return e.DID
}

// IncreaseContractVersionEvent is emitted when change requests for contract merged
type IncreaseContractVersionEvent struct {
	DID                string             `json:"did"`
	HolderDID          string             `json:"holder_did"`
	OldContractVersion int                `json:"old_contract_version"`
	NewContractVersion int                `json:"new_contract_version"`
	SubmittedBy        string             `json:"submitted_by"`
	OccurredAt         time.Time          `json:"occurred_at"`
	UserRoles          userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e IncreaseContractVersionEvent) EventType() string {
	return eventtype.IncreaseContractVersion.String()
}

// GetDID implements the Event interface.
func (e IncreaseContractVersionEvent) GetDID() string {
	return e.DID
}

// ContractExpired is emitted when change requests for contract merged
type ContractExpired struct {
	DID             string                             `json:"did"`
	HolderDID       string                             `json:"holder_did"`
	ContractVersion int                                `json:"old_contract_version"`
	ExpPolicy       *expirationpolicy.ExpirationPolicy `json:"exp_policy"`
	OccurredAt      time.Time                          `json:"occurred_at"`
	State           contractstate.ContractState        `json:"state"`
	UserRoles       userrole.UserRoles                 `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ContractExpired) EventType() string {
	return eventtype.ContractExpired.String()
}

// GetDID implements the Event interface.
func (e ContractExpired) GetDID() string {
	return e.DID
}

// RetrieveAllTemplatesEvent is emitted when template data is retrieved.
type RetrieveAllTemplatesEvent struct {
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e RetrieveAllTemplatesEvent) EventType() string {
	return eventtype.RetrieveAllTemplates.String()
}

// GetDID implements the Event interface.
func (e RetrieveAllTemplatesEvent) GetDID() string {
	return "*"
}

// SearchEvent is emitted when contract data is searched.
type SearchEvent struct {
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e SearchEvent) EventType() string {
	return eventtype.Search.String()
}

// GetDID implements the Event interface.
func (e SearchEvent) GetDID() string {
	return "*"
}
