// Package eventtype enumerates template catalogue integration event type strings.
package eventtype

type EventType string

const (
	RetrieveAll  EventType = "RETRIEVE_ALL_TEMPLATE_CATALOGUE"
	RetrieveByID EventType = "RETRIEVE_TEMPLATE_CATALOGUE_BY_ID"
	Search       EventType = "SEARCH_TEMPLATE_CATALOGUE"
)

func (s EventType) String() string {
	return string(s)
}
