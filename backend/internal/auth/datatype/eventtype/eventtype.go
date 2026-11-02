package eventtype

type EventType string

const (
	PresentationSucceeded EventType = "OID4VP_PRESENTATION_SUCCEEDED"
	PresentationFailed    EventType = "OID4VP_PRESENTATION_FAILED"
	SessionEnded          EventType = "AUTH_SESSION_ENDED"

	MachineIdentityCreated       EventType = "MACHINE_IDENTITY_CREATED"
	MachineIdentityUpdated       EventType = "MACHINE_IDENTITY_UPDATED"
	MachineIdentityDeleted       EventType = "MACHINE_IDENTITY_DELETED"
	MachineIdentitySecretRotated EventType = "MACHINE_IDENTITY_SECRET_ROTATED"

	LockoutLifted EventType = "LOCKOUT_LIFTED"
)

func (t EventType) String() string {
	return string(t)
}
