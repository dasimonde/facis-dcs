package webhookplatform

import (
	"testing"

	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	treventtype "digital-contracting-service/internal/templaterepository/datatype/eventtype"
)

// The NATS bridge drops any event type absent from DCSEventMap, and the
// subscription endpoint rejects any event name absent from KnownEvents —
// both sides must agree for a lifecycle notification to be subscribable
// and deliverable.
func TestDCSEventMapTargetsAreSubscribable(t *testing.T) {
	known := make(map[string]bool, len(KnownEvents))
	for _, e := range KnownEvents {
		known[e.Name] = true
	}
	for dcsType, webhookEvent := range DCSEventMap {
		if !known[webhookEvent] {
			t.Errorf("DCSEventMap[%q] = %q is not in KnownEvents, so no subscriber could ever receive it", dcsType, webhookEvent)
		}
	}
}

// DCS-FR-CSA-20 / DCS-FR-CWE-10: the expiry cron's ContractExpired event
// must reach webhook subscribers as an API-delivered alert.
func TestContractExpiredIsMapped(t *testing.T) {
	got, ok := DCSEventMap[cweeventtype.ContractExpired.String()]
	if !ok {
		t.Fatalf("DCSEventMap has no entry for %q", cweeventtype.ContractExpired.String())
	}
	if got != "contract.expired" {
		t.Fatalf("DCSEventMap[%q] = %q, want %q", cweeventtype.ContractExpired.String(), got, "contract.expired")
	}
}

// DCS-FR-TR-22: archiving a registered/published template deprecates it,
// and Template Users must be notified of the deprecation.
func TestTemplateDeprecatedIsMapped(t *testing.T) {
	got, ok := DCSEventMap[treventtype.Archive.String()]
	if !ok {
		t.Fatalf("DCSEventMap has no entry for %q", treventtype.Archive.String())
	}
	if got != "template.deprecated" {
		t.Fatalf("DCSEventMap[%q] = %q, want %q", treventtype.Archive.String(), got, "template.deprecated")
	}
}
