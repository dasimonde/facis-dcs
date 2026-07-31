// Package event defines the template repository's domain events, each
// implementing base/event.Event (EventType/GetDID). Handlers pass these to
// base/event.Create in the same DB transaction as their mutation;
// base/event.OutboxProcessor anchors them into the tamper-evident audit
// trail and republishes them on NATS.
package event

import (
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/templaterepository/datatype/actionflag"
	"digital-contracting-service/internal/templaterepository/datatype/eventtype"
)

// CreateEvent is emitted when a new contract template is created.
type CreateEvent struct {
	DID          string             `json:"did"`
	CreatedBy    string             `json:"created_by"`
	UpdatedAt    time.Time          `json:"updated_at"`
	Name         *string            `json:"name"`
	Description  *string            `json:"description"`
	TemplateData *datatype.JSON     `json:"template_data"`
	OccurredAt   time.Time          `json:"occurred_at"`
	HolderDID    string             `json:"holder_did"`
	UserRoles    userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e CreateEvent) EventType() string {
	return eventtype.Create.String()
}

// GetDID implements the Event interface.
func (e CreateEvent) GetDID() string {
	return e.DID
}

// CopyEvent is emitted when a new contract template is created.
type CopyEvent struct {
	NewDID     string             `json:"did"`
	CopyDID    string             `json:"copy_did"`
	CopiedBy   string             `json:"copied_by"`
	NewVersion int                `json:"new_version"`
	OccurredAt time.Time          `json:"occurred_at"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e CopyEvent) EventType() string {
	return eventtype.Copy.String()
}

// GetDID implements the Event interface.
func (e CopyEvent) GetDID() string {
	return e.CopyDID
}

// SubmitEvent is emitted when a template is submitted
type SubmitEvent struct {
	DID           string                 `json:"did"`
	Version       int                    `json:"version"`
	PreviousState string                 `json:"previous_state"`
	NewState      string                 `json:"new_state"`
	SubmittedBy   string                 `json:"submitted_by"`
	ActionFlag    *actionflag.ActionFlag `json:"action_flag"`
	Comments      []string               `json:"comments,omitempty"`
	OccurredAt    time.Time              `json:"occurred_at"`
	Responsible   *any                   `json:"responsible,omitempty"`
	HolderDID     string                 `json:"holder_did"`
	UserRoles     userrole.UserRoles     `json:"user_roles"`
}

// EventType implements the Event interface.
func (e SubmitEvent) EventType() string {
	return eventtype.Submit.String()
}

// GetDID implements the Event interface.
func (e SubmitEvent) GetDID() string {
	return e.DID
}

// ApproveEvent is emitted when a template is approved.
type ApproveEvent struct {
	DID           string             `json:"did"`
	Version       int                `json:"version"`
	ApprovedBy    string             `json:"approved_by"`
	DecisionNotes []string           `json:"decision_notes,omitempty"`
	OccurredAt    time.Time          `json:"occurred_at"`
	HolderDID     string             `json:"holder_did"`
	UserRoles     userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ApproveEvent) EventType() string {
	return eventtype.Approve.String()
}

// GetDID implements the Event interface.
func (e ApproveEvent) GetDID() string {
	return e.DID
}

// RejectEvent is emitted when a template is rejected.
type RejectEvent struct {
	DID        string             `json:"did"`
	Version    int                `json:"version"`
	RejectedBy string             `json:"rejected_by"`
	Reason     string             `json:"reason"`
	OccurredAt time.Time          `json:"occurred_at"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e RejectEvent) EventType() string {
	return eventtype.Reject.String()
}

// GetDID implements the Event interface.
func (e RejectEvent) GetDID() string {
	return e.DID
}

// VerifyEvent is emitted when a template is verified.
type VerifyEvent struct {
	DID        string             `json:"did"`
	Version    int                `json:"version"`
	VerifiedBy string             `json:"verified_by"`
	OccurredAt time.Time          `json:"occurred_at"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e VerifyEvent) EventType() string {
	return eventtype.Verify.String()
}

// GetDID implements the Event interface.
func (e VerifyEvent) GetDID() string {
	return e.DID
}

// UpdateEvent is emitted when template data is updated.
type UpdateEvent struct {
	DID             string             `json:"did"`
	UpdatedBy       string             `json:"updated_by"`
	OldName         *string            `json:"old_name,omitempty"`
	NewName         *string            `json:"new_name,omitempty"`
	OldDescription  *string            `json:"old_description,omitempty"`
	NewDescription  *string            `json:"new_description,omitempty"`
	OldTemplateData *datatype.JSON     `json:"old_template_data,omitempty"`
	NewTemplateData *datatype.JSON     `json:"new_template_data,omitempty"`
	OccurredAt      time.Time          `json:"occurred_at"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e UpdateEvent) EventType() string {
	return eventtype.Update.String()
}

// GetDID implements the Event interface.
func (e UpdateEvent) GetDID() string {
	return e.DID
}

// UpdateManageEvent is emitted when template data is updated.
type UpdateManageEvent struct {
	DID             string             `json:"did"`
	UpdatedBy       string             `json:"updated_by"`
	OldName         *string            `json:"old_name,omitempty"`
	NewName         *string            `json:"new_name,omitempty"`
	OldDescription  *string            `json:"old_description,omitempty"`
	NewDescription  *string            `json:"new_description,omitempty"`
	OldTemplateData *datatype.JSON     `json:"old_template_data,omitempty"`
	NewTemplateData *datatype.JSON     `json:"new_template_data,omitempty"`
	OccurredAt      time.Time          `json:"occurred_at"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e UpdateManageEvent) EventType() string {
	return eventtype.Update.String()
}

// GetDID implements the Event interface.
func (e UpdateManageEvent) GetDID() string {
	return e.DID
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

// RetrieveAllEvent is emitted when template data is retrieved.
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

// RetrieveByIDEvent is emitted when template data is retrieved.
type RetrieveByIDEvent struct {
	DID         string             `json:"did"`
	Version     int                `json:"version"`
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

// ArchiveEvent is emitted when template data is archived.
type ArchiveEvent struct {
	DID        string             `json:"did"`
	Version    int                `json:"version"`
	ArchivedBy string             `json:"archived_by"`
	OccurredAt time.Time          `json:"occurred_at"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ArchiveEvent) EventType() string {
	return eventtype.Archive.String()
}

// GetDID implements the Event interface.
func (e ArchiveEvent) GetDID() string {
	return e.DID
}

// RegisterEvent is emitted when template data is registered.
type RegisterEvent struct {
	DID           string             `json:"did"`
	RegisteredBy  string             `json:"registered_by"`
	UpdatedAt     time.Time          `json:"updated_at"`
	Name          *string            `json:"name"`
	Description   *string            `json:"description"`
	TemplateData  *datatype.JSON     `json:"template_data"`
	SourceDID     string             `json:"source_did"`
	SourceVersion int                `json:"source_version"`
	OccurredAt    time.Time          `json:"occurred_at"`
	HolderDID     string             `json:"holder_did"`
	UserRoles     userrole.UserRoles `json:"user_roles"`
	PreviousState *string            `json:"previous_state,omitempty"`
	NewState      *string            `json:"new_state,omitempty"`
}

// EventType implements the Event interface.
func (e RegisterEvent) EventType() string {
	return eventtype.Register.String()
}

// GetDID implements the Event interface.
func (e RegisterEvent) GetDID() string {
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

// PublishEvent is emitted when template data is published to the Federated Catalogue.
type PublishEvent struct {
	DID         string             `json:"did"`
	Version     int                `json:"version"`
	PublishedBy string             `json:"published_by"`
	HolderDID   string             `json:"holder_did"`
	OccurredAt  time.Time          `json:"occurred_at"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e PublishEvent) EventType() string {
	return eventtype.Publish.String()
}

// GetDID implements the Event interface.
func (e PublishEvent) GetDID() string {
	return e.DID
}
