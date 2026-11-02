package workflowgate

import (
	"slices"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp"
)

// DCS-NFR-SEC-18: the PID request asks for every claim this gate reads, and
// the DCS keeps no other (oid4vp.PIDClaimPaths). A claim read here but not
// requested would never be there to find.
func TestEveryPIDClaimTheGateReadsIsRequested(t *testing.T) {
	requested := func(path ...string) bool {
		return slices.ContainsFunc(oid4vp.PIDClaimPaths, func(p []string) bool { return slices.Equal(p, path) })
	}
	for _, name := range append(slices.Clone(pidClaimNames), "given_name") {
		if !requested(name) {
			t.Errorf("the gate reads PID claim %q, which the PID request does not ask for", name)
		}
	}
	if !requested("address", "street_address") {
		t.Error("the gate reads address.street_address, which the PID request does not ask for")
	}
}
