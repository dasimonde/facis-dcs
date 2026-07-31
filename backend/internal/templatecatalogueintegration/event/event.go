// Package event defines template catalogue integration domain events for the audit trail.
package event

import (
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"
	catalogueeventtype "digital-contracting-service/internal/templatecatalogueintegration/datatype/eventtype"
)

// RetrieveAllEvent is emitted when catalogue templates are listed.
type RetrieveAllEvent struct {
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

func (e RetrieveAllEvent) EventType() string {
	return catalogueeventtype.RetrieveAll.String()
}

func (e RetrieveAllEvent) GetDID() string {
	return "*"
}

// RetrieveByIDEvent is emitted when a catalogue template is retrieved by DID and version.
type RetrieveByIDEvent struct {
	DID         string             `json:"did"`
	Version     int                `json:"version"`
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

func (e RetrieveByIDEvent) EventType() string {
	return catalogueeventtype.RetrieveByID.String()
}

func (e RetrieveByIDEvent) GetDID() string {
	return e.DID
}

// SearchEvent is emitted when catalogue templates are searched.
type SearchEvent struct {
	RetrievedBy string             `json:"retrieved_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

func (e SearchEvent) EventType() string {
	return catalogueeventtype.Search.String()
}

func (e SearchEvent) GetDID() string {
	return "*"
}
