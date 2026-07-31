package eventtype

type EventType string

const (
	PresentationSucceeded EventType = "OID4VP_PRESENTATION_SUCCEEDED"
	PresentationFailed    EventType = "OID4VP_PRESENTATION_FAILED"
)

func (t EventType) String() string {
	return string(t)
}
