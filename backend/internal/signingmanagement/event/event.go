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

// ValidateEvent is emitted when a signature is validated.
type ValidateEvent struct {
	DID             string             `json:"did"`
	ContractVersion int                `json:"contract_version,omitempty"`
	ValidatedBy     string             `json:"validated_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ValidateEvent) EventType() string {
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
}

// EventType implements the Event interface.
func (e AuditEvt) EventType() string {
	return eventtype.Audit.String()
}

// GetDID implements the Event interface.
func (e AuditEvt) GetDID() string {
	return e.DID
}

// RevokeEvent is emitted when a signature is revoked
type RevokeEvent struct {
	DID             string             `json:"did"`
	ContractVersion int                `json:"contract_version,omitempty"`
	Reason          string             `json:"reason"`
	RevokedBy       string             `json:"revoked_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e RevokeEvent) EventType() string {
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

// ApplyEvent is emitted when contract is reviewed.
type ApplyEvent struct {
	DID             string             `json:"did"`
	ContractVersion int                `json:"contract_version"`
	AppliedBy       string             `json:"applied_by"`
	OccurredAt      time.Time          `json:"occurred_at"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
	CredentialType  string             `json:"credential_type"`
}

// EventType implements the Event interface.
func (e ApplyEvent) EventType() string {
	return eventtype.Applied.String()
}

// GetDID implements the Event interface.
func (e ApplyEvent) GetDID() string {
	return e.DID
}
