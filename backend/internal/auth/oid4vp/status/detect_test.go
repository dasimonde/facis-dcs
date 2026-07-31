package status_test

import (
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"

	"github.com/stretchr/testify/assert"
)

func TestIsXFSCStatusListJSON(t *testing.T) {
	xfscJSON := []byte(`{"tenantId":"default","listId":1,"list":"H4sIAAAAAA=="}`)
	ietfJWT := []byte(`eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ1cmkifQ.sig`)
	w3cJWT := []byte(`eyJhbGciOiJFUzI1NiJ9.eyJjcmVkZW50aWFsU3ViamVjdCI6e319.sig`)

	assert.True(t, status.IsXFSCStatusListJSON(xfscJSON))
	assert.False(t, status.IsXFSCStatusListJSON(ietfJWT))
	assert.False(t, status.IsXFSCStatusListJSON(w3cJWT))
	assert.False(t, status.IsXFSCStatusListJSON([]byte(`{"tenantId":"","listId":1,"list":"x"}`)))
}

func TestValidateIETFStatusListJWTHeader(t *testing.T) {
	assert.NoError(t, status.ValidateIETFStatusListJWTHeader(map[string]any{"typ": "statuslist+jwt"}))
	assert.Error(t, status.ValidateIETFStatusListJWTHeader(map[string]any{"typ": "JWT"}))
}

func TestIsXFSCStatusListJWTType(t *testing.T) {
	assert.True(t, status.IsXFSCStatusListJWTType(map[string]any{"typ": "statuslist+jwt"}))
	assert.True(t, status.IsXFSCStatusListJWTType(map[string]any{"typ": "JWT"}))
	assert.False(t, status.IsXFSCStatusListJWTType(map[string]any{"typ": "vc+jwt"}))
}
