// Package eventtype enumerates the template repository's own event type
// strings, used as the EventType() of templaterepository/event structs.
package eventtype

type EventType string

const (
	Create       EventType = "CREATE_CONTRACT_TEMPLATE"
	Copy         EventType = "COPY_CONTRACT_TEMPLATE"
	Submit       EventType = "SUBMIT_CONTRACT_TEMPLATE"
	Approve      EventType = "APPROVE_CONTRACT_TEMPLATE"
	Reject       EventType = "REJECT_CONTRACT_TEMPLATE"
	Verify       EventType = "VERIFY_CONTRACT_TEMPLATE"
	Update       EventType = "UPDATE_CONTRACT_TEMPLATE"
	RetrieveAll  EventType = "RETRIEVE_ALL_CONTRACT_TEMPLATES"
	RetrieveByID EventType = "RETRIEVE_CONTRACT_TEMPLATE_BY_ID"
	Search       EventType = "SEARCH_CONTRACT_TEMPLATE"
	Archive      EventType = "ARCHIVE_CONTRACT_TEMPLATE"
	Register     EventType = "REGISTER_CONTRACT_TEMPLATE"
	Audit        EventType = "AUDIT_CONTRACT_TEMPLATE"
	Publish      EventType = "PUBLISH_CONTRACT_TEMPLATE"
)

func (s EventType) String() string {
	return string(s)
}
