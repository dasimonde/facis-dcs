package event

import (
	"time"

	"digital-contracting-service/internal/auth/datatype/eventtype"
)

// PresentationSucceededEvent is emitted when a wallet presentation completes login.
type PresentationSucceededEvent struct {
	PresentationState string    `json:"presentation_state"`
	SubjectDID        string    `json:"subject_did"`
	ParticipantDID    string    `json:"participant_did"`
	Roles             []string  `json:"roles"`
	OccurredAt        time.Time `json:"occurred_at"`
}

func (e PresentationSucceededEvent) EventType() string {
	return eventtype.PresentationSucceeded.String()
}

func (e PresentationSucceededEvent) GetDID() string {
	return e.PresentationState
}

// PresentationFailedEvent is emitted when OID4VP presentation verification or login is denied.
type PresentationFailedEvent struct {
	PresentationState string    `json:"presentation_state"`
	SubjectDID        string    `json:"subject_did,omitempty"`
	ParticipantDID    string    `json:"participant_did,omitempty"`
	Roles             []string  `json:"roles,omitempty"`
	ErrorMessage      string    `json:"error_message"`
	OccurredAt        time.Time `json:"occurred_at"`
}

func (e PresentationFailedEvent) EventType() string {
	return eventtype.PresentationFailed.String()
}

func (e PresentationFailedEvent) GetDID() string {
	return e.PresentationState
}
