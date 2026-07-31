package status_test

import (
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"

	"github.com/stretchr/testify/assert"
)

func TestFetchOptsForReference(t *testing.T) {
	assert.Equal(t, fetch.RequestOpts{Accept: status.IETFStatusListAccept}, status.FetchOptsForReference(status.Reference{
		Mechanism: status.MechanismIETFToken,
	}))
	assert.Equal(t, fetch.RequestOpts{}, status.FetchOptsForReference(status.Reference{
		Mechanism: status.MechanismW3CBitstring,
	}))
}

func TestSelectMechanismFromResponse(t *testing.T) {
	ietfRef := status.Reference{
		Mechanism: status.MechanismIETFToken,
		URI:       "https://issuer.example/status/ietf",
	}
	w3cRef := status.Reference{
		Mechanism: status.MechanismW3CBitstring,
		URI:       "https://issuer.example/status/w3c",
	}
	xfscJSON := []byte(`{"tenantId":"default","listId":1,"list":"H4sIAAAAAA=="}`)
	ietfJWT := []byte(`eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ1cmkifQ.sig`)
	w3cJWT := []byte(`eyJhbGciOiJFUzI1NiJ9.eyJjcmVkZW50aWFsU3ViamVjdCI6e319.sig`)

	assert.Equal(t, status.MechanismXFSC, status.SelectMechanismFromResponse(ietfRef, fetch.Response{Body: xfscJSON}))
	assert.Equal(t, status.MechanismIETFToken, status.SelectMechanismFromResponse(ietfRef, fetch.Response{
		ContentType: "application/statuslist+jwt",
		Body:        ietfJWT,
	}))
	assert.Equal(t, status.MechanismW3CBitstring, status.SelectMechanismFromResponse(w3cRef, fetch.Response{
		ContentType: "application/vc+jwt",
		Body:        w3cJWT,
	}))
}

func TestSelectMechanismFromResponse_XFSCPathDoesNotAutoRouteWithoutXFSCBody(t *testing.T) {
	ref := status.Reference{
		Mechanism: status.MechanismIETFToken,
		URI:       "http://localhost:30821/v1/tenants/default/status/1",
	}
	ietfJWT := []byte(`eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ1cmkifQ.sig`)

	assert.Equal(t, status.MechanismIETFToken, status.SelectMechanismFromResponse(ref, fetch.Response{
		ContentType: "application/statuslist+jwt",
		Body:        ietfJWT,
	}))
}

func TestSelectMechanismFromResponse_UnknownResponseKeepsReferenceMechanism(t *testing.T) {
	ref := status.Reference{
		Mechanism: status.MechanismW3CBitstring,
		URI:       "https://issuer.example/status/w3c",
	}

	assert.Equal(t, status.MechanismW3CBitstring, status.SelectMechanismFromResponse(ref, fetch.Response{
		Body: []byte("not-json"),
	}))
}
