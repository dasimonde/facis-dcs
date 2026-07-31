package status_test

import (
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"

	"github.com/stretchr/testify/assert"
)

func TestStatusListSubjectMatchesURI(t *testing.T) {
	did := "eyJjcnYiOiJQLTI1NiIsImV4dCI6dHJ1ZSwia2V5X29wcyI6WyJ2ZXJpZnkiXSwia3R5IjoiRUMiLCJ4IjoiakV2UlAtcHhGNFhDQlZMaEVyV1Zvd3RCZWU5NGpZNE95VmQycjJYd2JndyIsInkiOiJ1YTNDQ0lNUlJZNk13dE1WSXFhdlFndk5BNFh5eWNWWmE3cjFIN01MSENJIn0"
	ref := "http://localhost:5175/statuslist/v1/did%3Ajwk%3A" + did + "/statusList"
	sub := "http://localhost:5175/statuslist/v1/did:jwk:" + did + "/statusList"

	assert.True(t, status.SubjectMatchesURI(sub, ref))
	assert.True(t, status.SubjectMatchesURI(ref, ref))
	assert.False(t, status.SubjectMatchesURI("http://localhost:5175/status/ietf/token/wrong-sub", "http://localhost:5175/status/ietf/token/bad-sub"))
}
