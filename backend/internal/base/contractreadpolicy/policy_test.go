package contractreadpolicy

import (
	"context"
	"testing"

	"digital-contracting-service/internal/base/datatype"
)

const localPeer = "did:web:local"

func TestMayRead(t *testing.T) {
	party := datatype.JSON(`{"dcs:parties":[{"dcs:legalName":"Org A"}]}`)
	none := datatype.JSON(`{}`)
	user := Scope{Organization: "Org A", LocalPeer: localPeer}
	for _, tc := range []struct {
		name              string
		scope             Scope
		origin, createdBy string
		data              *datatype.JSON
		want              bool
	}{
		{"own organization", user, localPeer, "Org A", &none, true},
		{"named party", user, localPeer, "Org B", &party, true},
		{"foreign organization", user, localPeer, "Org B", &none, false},
		{"foreign without data", user, localPeer, "Org B", nil, false},
		{"received copy without parties", user, "did:web:peer", "did:web:peer", &none, false},
		{"received copy naming the organization", user, "did:web:peer", "did:web:peer", &party, true},
		{"received copy naming another organization", Scope{Organization: "Org C", LocalPeer: localPeer}, "did:web:peer", "did:web:peer", &party, false},
		{"no organization", Scope{LocalPeer: localPeer}, localPeer, "", &party, false},
		{"system client acting for the instance", Scope{Organization: localPeer, LocalPeer: localPeer}, localPeer, "Org B", &none, true},
		{"system client acting for the instance reads a received copy", Scope{Organization: localPeer, LocalPeer: localPeer}, "did:web:peer", "did:web:peer", &none, true},
		{"instance DID without a known local peer", Scope{Organization: localPeer}, localPeer, "Org B", &none, false},
		{"system scope", Scope{system: true}, localPeer, "Org B", &none, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.scope.MayRead(tc.origin, tc.createdBy, tc.data); got != tc.want {
				t.Fatalf("MayRead = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPredicateDecision(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want rowFilter
	}{
		{"no scope", ctx, noRows},
		{"user scope", WithScope(ctx, Scope{Organization: "Org A", LocalPeer: localPeer}), partyRows},
		{"system client acting for the instance", WithScope(ctx, Scope{Organization: localPeer, LocalPeer: localPeer}), allRows},
		{"system scope", WithSystemScope(ctx), allRows},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, got := decide(tc.ctx); got != tc.want {
				t.Fatalf("decide = %v, want %v", got, tc.want)
			}
		})
	}
}
