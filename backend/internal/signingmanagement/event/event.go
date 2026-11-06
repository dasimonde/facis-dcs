package event

import (
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/signingmanagement/datatype/eventtype"
)

// RetrieveByIDEvent is emitted when contract data is retrieved.
type RetrieveByIDEvent struct {
	DID         string             `json:"did"`
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	HolderDID   string             `json:"holder_did"`
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

// RetrieveForSigningEvent is emitted when the document to be signed is handed
// out (DCS-FR-SM-15). DocumentSHA256 is the digest the signed request object
// names, so the wallet validates the document it receives against it.
type RetrieveForSigningEvent struct {
	DID            string             `json:"did"`
	SignerDID      string             `json:"signer_did"`
	CeremonyID     string             `json:"ceremony_id"`
	FieldName      string             `json:"field_name,omitempty"`
	DocumentSHA256 string             `json:"document_sha256"`
	Channel        string             `json:"channel"`
	RetrievedBy    string             `json:"retrieved_by,omitempty"`
	OccurredAt     time.Time          `json:"occurred_at"`
	HolderDID      string             `json:"holder_did,omitempty"`
	UserRoles      userrole.UserRoles `json:"user_roles,omitempty"`
}

// Channels of a RetrieveForSigningEvent.
const (
	ChannelPrepare        = "PREPARE"
	ChannelWalletDocument = "WALLET_DOCUMENT"
)

// EventType implements the Event interface.
func (e RetrieveForSigningEvent) EventType() string {
	return eventtype.RetrieveForSigning.String()
}

// GetDID implements the Event interface.
func (e RetrieveForSigningEvent) GetDID() string {
	return e.DID
}

// RetrieveAllEvent is emitted when contract data is retrieved.
type RetrieveAllEvent struct {
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e RetrieveAllEvent) EventType() string {
	return eventtype.RetrieveAll.String()
}

// GetDID implements the Event interface.
func (e RetrieveAllEvent) GetDID() string {
	return "*"
}

// SearchEvent is emitted when template data is searched.
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

// Outcome values of an Apply, Validate or Revoke audit entry (DCS-FR-SM-19).
const (
	OutcomeCompleted = "COMPLETED"
	OutcomeFailed    = "FAILED"
)

// ValidateEvent is emitted when a signature validation completes or fails.
// Findings is the validation result; Error is set only for a failed attempt.
type ValidateEvent struct {
	DID             string             `json:"did"`
	ContractVersion int                `json:"contract_version,omitempty"`
	SignerDIDs      []string           `json:"signer_dids"`
	Outcome         string             `json:"outcome"`
	Findings        []string           `json:"findings"`
	Error           string             `json:"error,omitempty"`
	ValidatedBy     string             `json:"validated_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface. A failed attempt has its own type.
func (e ValidateEvent) EventType() string {
	if e.Outcome == OutcomeFailed {
		return eventtype.ValidateFailed.String()
	}
	return eventtype.Validate.String()
}

// GetDID implements the Event interface.
func (e ValidateEvent) GetDID() string {
	return e.DID
}

// VerifyEvent is emitted when a signature is validated.
type VerifyEvent struct {
	DID             string             `json:"did"`
	ContractVersion int                `json:"contract_version,omitempty"`
	VerifiedBy      string             `json:"verified_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e VerifyEvent) EventType() string {
	return eventtype.Validate.String()
}

// GetDID implements the Event interface.
func (e VerifyEvent) GetDID() string {
	return e.DID
}

// AuditEvt is emitted when template data is registered.
type AuditEvt struct {
	DID           string                      `json:"did"`
	AuditedBy     string                      `json:"audited_by"`
	OccurredAt    time.Time                   `json:"occurred_at"`
	ComponentType componenttype.ComponentType `json:"component_type"`
	HolderDID     string                      `json:"holder_did"`
	UserRoles     userrole.UserRoles          `json:"user_roles"`
	Justification string                      `json:"justification"`
}

// EventType implements the Event interface.
func (e AuditEvt) EventType() string {
	return eventtype.Audit.String()
}

// GetDID implements the Event interface.
func (e AuditEvt) GetDID() string {
	return e.DID
}

// RevokeEvent is emitted when a signature revocation completes or fails.
// Error is set only for a failed attempt.
type RevokeEvent struct {
	DID             string             `json:"did"`
	ContractVersion int                `json:"contract_version,omitempty"`
	SignerDID       string             `json:"signer_did"`
	Outcome         string             `json:"outcome"`
	Reason          string             `json:"reason"`
	Error           string             `json:"error,omitempty"`
	RevokedBy       string             `json:"revoked_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface. A failed attempt has its own type:
// dcstodcs ships the contract to peers on REVOKE_SIGNATURE.
func (e RevokeEvent) EventType() string {
	if e.Outcome == OutcomeFailed {
		return eventtype.RevokeFailed.String()
	}
	return eventtype.Revoke.String()
}

// GetDID implements the Event interface.
func (e RevokeEvent) GetDID() string {
	return e.DID
}

// ComplianceValidationEvent is emitted when compliance check ist started
type ComplianceValidationEvent struct {
	DID             string             `json:"did"`
	ContractVersion int                `json:"contract_version,omitempty"`
	CheckedBy       string             `json:"checked_by"`
	Findings        []string           `json:"findings"`
	OccurredAt      time.Time          `json:"occurred_at"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ComplianceValidationEvent) EventType() string {
	return eventtype.ComplianceValidation.String()
}

// GetDID implements the Event interface.
func (e ComplianceValidationEvent) GetDID() string {
	return e.DID
}

// Steps of a signing attempt recorded in an ApplyEvent.
const (
	StepPrepare        = "PREPARE"
	StepSubmit         = "SUBMIT"
	StepWalletCallback = "WALLET_CALLBACK"
	// StepPresentation is the PID and PoA presentation of a signing ceremony.
	StepPresentation = "PRESENTATION"
)

// ApplyEvent is emitted when a signature is applied, and when a signing
// attempt is refused (DCS-FR-SM-19). Error is set only for a refused attempt.
type ApplyEvent struct {
	DID             string             `json:"did"`
	ContractVersion int                `json:"contract_version"`
	AppliedBy       string             `json:"applied_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
	CredentialType  string             `json:"credential_type"`
	SignerDID       string             `json:"signer_did"`
	FieldName       string             `json:"field_name,omitempty"`
	CeremonyID      string             `json:"ceremony_id,omitempty"`
	Step            string             `json:"step"`
	Outcome         string             `json:"outcome"`
	Error           string             `json:"error,omitempty"`
}

// EventType implements the Event interface. A refused attempt has its own
// type, so it does not trigger what an applied signature triggers.
func (e ApplyEvent) EventType() string {
	if e.Outcome == OutcomeFailed {
		return eventtype.ApplyFailed.String()
	}
	return eventtype.Applied.String()
}

// GetDID implements the Event interface.
func (e ApplyEvent) GetDID() string {
	return e.DID
}
