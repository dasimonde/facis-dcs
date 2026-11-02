package event

import (
	"testing"

	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	smeventtype "digital-contracting-service/internal/signingmanagement/datatype/eventtype"
)

// Create and Renew each start a new contract instance in DRAFT. Its PDF must
// render on that event, not wait for the retry sweep.
func TestANewContractInstanceIsRenderedOnItsEvent(t *testing.T) {
	for _, typ := range []cweeventtype.EventType{cweeventtype.Create, cweeventtype.Renew} {
		if !contractLifecycleEventTypes[typ.String()] {
			t.Errorf("%s must trigger PDF regeneration", typ)
		}
	}
}

// Revoke, terminate and expiry are the events after which a signed PDF gets its
// post-signing lifecycle manifest appended.
func TestPostSigningLifecycleEventsReachTheRegenerator(t *testing.T) {
	for _, typ := range []string{smeventtype.Revoke.String(), cweeventtype.Terminate.String(), cweeventtype.ContractExpired.String()} {
		if !contractLifecycleEventTypes[typ] {
			t.Errorf("%s must reach the regenerator", typ)
		}
	}
}
