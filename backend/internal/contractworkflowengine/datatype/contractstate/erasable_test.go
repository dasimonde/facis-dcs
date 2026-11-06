package contractstate

import "testing"

func TestOnlyContractsOutOfForceAreErasable(t *testing.T) {
	erasable := map[ContractState]bool{Terminated: true, Expired: true, Revoked: true, Rejected: true, Withdrawn: true}
	for _, state := range All() {
		if got := state.Erasable(); got != erasable[state] {
			t.Errorf("%s.Erasable() = %v, want %v", state, got, erasable[state])
		}
	}
	if !ContractState("terminated").Erasable() {
		t.Error("the predicate must not depend on the letter case of the stored state")
	}
}
