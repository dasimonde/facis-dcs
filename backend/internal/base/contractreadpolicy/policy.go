// Package contractreadpolicy shares organization-scoped contract read authorization.
package contractreadpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"digital-contracting-service/internal/base/datatype"
)

var ErrDenied = errors.New("not authorized to access this contract")

type Scope struct {
	Organization string
	LocalPeer    string
	system       bool
}
type scopeKey struct{}

func WithScope(ctx context.Context, scope Scope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// WithSystemScope marks a read that serves no user request, such as a background job or a peer sync.
// Scoped repository reads return no rows for a context without any scope.
func WithSystemScope(ctx context.Context) context.Context {
	return WithScope(ctx, Scope{system: true})
}
func FromContext(ctx context.Context) (Scope, bool) {
	scope, ok := ctx.Value(scopeKey{}).(Scope)
	return scope, ok
}

// ActsForInstance reports a system client whose organization is this
// instance's DID: it reads the contracts of every organization on the
// instance. A role alone does not cross organizations, so an Auditor, a
// System Administrator or a Sys.* client of an organization reads that
// organization's contracts only, as for writes (ADR-27).
func (s Scope) ActsForInstance() bool {
	return s.LocalPeer != "" && s.Organization == s.LocalPeer
}

// MayRead admits the creating organization and the organizations named in
// dcs:parties. A copy received from a peer has the peer's DID as creator, so on
// this instance only the organizations its sender named may read it; a copy
// that names none is readable only by a client that acts for the instance.
func (s Scope) MayRead(origin, createdBy string, raw *datatype.JSON) bool {
	if s.system || s.ActsForInstance() {
		return true
	}
	if s.Organization == "" {
		return false
	}
	if s.Organization == createdBy {
		return true
	}
	if raw == nil {
		return false
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal(*raw, &doc) != nil {
		return false
	}
	var parties []json.RawMessage
	if json.Unmarshal(doc["dcs:parties"], &parties) != nil {
		return false
	}
	for _, party := range parties {
		var node map[string]any
		if json.Unmarshal(party, &node) == nil && node["dcs:legalName"] == s.Organization {
			return true
		}
	}
	return false
}

// SQL uses the current trusted contract row, including when listing frozen archive metadata.
// didExpression is a repository-owned SQL identifier, never request input.
func SQL(ctx context.Context, didExpression string, params *[]any) string {
	s, rows := decide(ctx)
	switch rows {
	case noRows:
		return "FALSE"
	case allRows:
		return "TRUE"
	}
	n := len(*params) + 1
	*params = append(*params, s.Organization)
	return fmt.Sprintf(`EXISTS (SELECT 1 FROM contracts_effective read_contract WHERE read_contract.did = %s AND (
 $%d <> '' AND (read_contract.created_by = $%d OR EXISTS (
 SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(read_contract.contract_data->'dcs:parties') = 'array' THEN read_contract.contract_data->'dcs:parties' ELSE '[]'::jsonb END) party
 WHERE jsonb_typeof(party->'dcs:legalName') = 'string' AND party->>'dcs:legalName' = $%d))))`, didExpression, n, n, n)
}

type rowFilter int

const (
	noRows rowFilter = iota
	allRows
	partyRows
)

func decide(ctx context.Context) (Scope, rowFilter) {
	s, scoped := FromContext(ctx)
	switch {
	case !scoped:
		return s, noRows
	case s.system || s.ActsForInstance():
		return s, allRows
	}
	return s, partyRows
}
