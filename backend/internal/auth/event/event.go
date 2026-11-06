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

// SessionEndedEvent is emitted when the DCS ends a login session at token
// refresh, for example because the PoA credential of the session is revoked.
type SessionEndedEvent struct {
	SubjectDID     string    `json:"subject_did"`
	ParticipantDID string    `json:"participant_did,omitempty"`
	Reason         string    `json:"reason"`
	OccurredAt     time.Time `json:"occurred_at"`
}

func (e SessionEndedEvent) EventType() string {
	return eventtype.SessionEnded.String()
}

func (e SessionEndedEvent) GetDID() string {
	return e.SubjectDID
}

// MachineIdentityChangedEvent is emitted when an administrator creates,
// changes, deletes or issues a new secret for a machine identity. It records
// the identity as it is after the change, and who made the change.
type MachineIdentityChangedEvent struct {
	Change              eventtype.EventType `json:"-"`
	IdentityID          string              `json:"identity_id,omitempty"`
	OAuthClientID       string              `json:"oauth_client_id"`
	Name                string              `json:"name,omitempty"`
	ParticipantDID      string              `json:"participant_did,omitempty"`
	Roles               []string            `json:"roles,omitempty"`
	Enabled             bool                `json:"enabled"`
	ActorDID            string              `json:"actor_did"`
	ActorParticipantDID string              `json:"actor_participant_did"`
	OccurredAt          time.Time           `json:"occurred_at"`
}

func (e MachineIdentityChangedEvent) EventType() string {
	return e.Change.String()
}

// GetDID keys the audit chain by the OAuth2 client, the identifier the
// machine caller's own access attempts are logged under.
func (e MachineIdentityChangedEvent) GetDID() string {
	return e.OAuthClientID
}

// LockoutLiftedEvent is emitted when an administrator ends a lockout before
// its time (FR-UC-01-4).
type LockoutLiftedEvent struct {
	LockoutKey          string    `json:"lockout_key"`
	ActorDID            string    `json:"actor_did"`
	ActorParticipantDID string    `json:"actor_participant_did"`
	OccurredAt          time.Time `json:"occurred_at"`
}

func (e LockoutLiftedEvent) EventType() string {
	return eventtype.LockoutLifted.String()
}

func (e LockoutLiftedEvent) GetDID() string {
	return e.LockoutKey
}
