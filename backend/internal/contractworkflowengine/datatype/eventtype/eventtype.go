// Package eventtype enumerates the contract workflow engine's own event
// type strings, used both as the EventType() of contractworkflowengine/event
// structs and to filter/interpret events consumed by dcstodcs
// (RemoteSyncRequest/RemoteActionRequestEvent are explicitly excluded from
// triggering a further peer sync, to avoid sync loops).
package eventtype

type EventType string

const (
	Create            EventType = "CREATE_CONTRACT"
	RemoteSync        EventType = "REMOTE_SYNC"
	RemoteSyncRequest EventType = "REMOTE_SYNC_REQUEST"
	Submit            EventType = "SUBMIT_CONTRACT"
	Negotiation       EventType = "NEGOTIATE_CONTRACT"
	// AcceptOffer records the counterparty accepting an inbound offer as-is,
	// which mints its negotiation task for the round and opens NEGOTIATION. It
	// changes no contract content, so it ships nothing to the peer.
	AcceptOffer   EventType = "ACCEPT_OFFER_CONTRACT"
	AcceptRespond EventType = "ACCEPT_RESPOND_CONTRACT"
	RejectRespond EventType = "REJECT_RESPOND_CONTRACT"
	// WithdrawRespond records the author retracting their own change proposal
	// before any counterparty accepted it.
	WithdrawRespond EventType = "WITHDRAW_RESPOND_CONTRACT"
	// NegotiationChangeSuperseded records that folding a round discarded the
	// content of a change request that stands at ACCEPTED, because a later
	// accepted request set the same fields (last-accepted-wins). Without it
	// the trail shows only the acceptance, and reads as agreement to wording
	// the contract never carried (DCS-IR-CWE-03).
	NegotiationChangeSuperseded EventType = "NEGOTIATION_CHANGE_SUPERSEDED"
	IncreaseContractVersion     EventType = "INCREASE_CONTRACT_VERSION"
	Approve                     EventType = "APPROVE_CONTRACT"
	Reject                      EventType = "REJECT_CONTRACT"
	Verify                      EventType = "VERIFY_CONTRACT"
	Update                      EventType = "UPDATE_CONTRACT"
	OutdatedPeer                EventType = "OUTDATED_PEER"
	RetrieveAll                 EventType = "RETRIEVE_ALL_CONTRACTS"
	RetrieveArchived            EventType = "RETRIEVE_ARCHIVED_CONTRACTS"
	StoreArchived               EventType = "STORE_ARCHIVED_CONTRACT"
	DeleteArchived              EventType = "DELETE_ARCHIVED_CONTRACT"
	AnnotateArchived            EventType = "ANNOTATE_ARCHIVED_CONTRACT"
	SearchArchived              EventType = "SEARCH_ARCHIVED_CONTRACTS"
	StoreArchiveEvidence        EventType = "STORE_ARCHIVE_EVIDENCE"
	StoreArchiveSection         EventType = "STORE_ARCHIVE_SECTION"
	// StoreExecutionProof records the proof of contract execution over every
	// signature, written when the last counterparty signature arrives after
	// the archive entry (DCS-FR-SM-10).
	StoreExecutionProof    EventType = "STORE_EXECUTION_PROOF"
	ArchiveOperationFailed EventType = "ARCHIVE_OPERATION_FAILED"
	RetrieveByID           EventType = "RETRIEVE_CONTRACT_BY_ID"
	AccessDenied           EventType = "CONTRACT_ACCESS_DENIED"
	RetrieveHistoryByDID   EventType = "RETRIEVE_CONTRACT_HISTORY_BY_DID"
	Search                 EventType = "SEARCH_CONTRACT"
	Review                 EventType = "REVIEW_CONTRACT"
	Audit                  EventType = "AUDIT_CONTRACT"
	Terminate              EventType = "TERMINATE_CONTRACT"
	// Activate records the deployment acknowledgement that moves a contract
	// from SIGNED to ACTIVE.
	Activate                 EventType = "ACTIVATE_CONTRACT"
	Renew                    EventType = "RENEW_CONTRACT"
	RecordEvidence           EventType = "RECORD_EVIDENCE"
	ContractExpired          EventType = "CONTRACT_EXPIRED"
	ContractRenewalDue       EventType = "CONTRACT_RENEWAL_DUE"
	RetrieveAllTemplates     EventType = "RETRIEVE_ALL_TEMPLATES"
	RemoteActionRequestEvent EventType = "REMOTE_ACTION_REQUEST"
	Offer                    EventType = "OFFER_CONTRACT"
	Withdraw                 EventType = "WITHDRAW_CONTRACT"
	// PublishCatalogue records that a contract's metadata was registered in
	// the Federated Catalogue (DCS-IR-CWE-10).
	PublishCatalogue EventType = "PUBLISH_CONTRACT_TO_CATALOGUE"
	// WithdrawCatalogue records that a contract's catalogue entry was revoked,
	// because the contract left the published states or was registered again
	// (DCS-IR-CWE-10, DCS-FR-CWE-23).
	WithdrawCatalogue EventType = "WITHDRAW_CONTRACT_FROM_CATALOGUE"
	// Export is emitted when a contract bundle (ZIP) is exported. An export
	// is a retrieval-class action and is recorded in the audit log
	// (FR-CSA-18). Kept as the bare "EXPORT" token so the audit trail
	// surfaces it verbatim.
	Export EventType = "EXPORT"
	// Revoke marks the invalidation of a signed contract via signature
	// revocation (FR-SM-20); no command emits it yet.
	Revoke EventType = "REVOKE_CONTRACT"

	// PDFRegenerated is emitted after the background regenerator has rebuilt and
	// stored a contract's PDF. The DCS-to-DCS synchronizer ships that PDF to the
	// counterparty on shippable transitions (ADR-13).
	PDFRegenerated EventType = "PDF_REGENERATED"

	// KeyShredded records the destruction of a contract's wrapped
	// content-encryption keys (DCS-NFR-SEC-13, DCS-NFR-COMP-03) — locally on
	// archive deletion or on an authenticated peer's erase request. Its body
	// carries no contract content and is anchored under the instance scope so
	// the destruction record survives the shredding it documents.
	KeyShredded EventType = "KEY_SHREDDED"

	// CreateContractTarget, UpdateContractTarget and DeleteContractTarget
	// record an administrator's change to a contract target system
	// (FR-UC-09-2). An update includes enabling and disabling the target.
	CreateContractTarget EventType = "CREATE_CONTRACT_TARGET"
	UpdateContractTarget EventType = "UPDATE_CONTRACT_TARGET"
	DeleteContractTarget EventType = "DELETE_CONTRACT_TARGET"
)

func (s EventType) String() string {
	return string(s)
}
