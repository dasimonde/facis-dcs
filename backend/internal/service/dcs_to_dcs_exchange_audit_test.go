package service

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base/pgtest"
)

// DCS-NFR-BR-08: every inbound peer exchange that completes leaves an audit
// event; a refused one does not.
func TestReceivedPeerExchangeIsAuditedOnlyWhenItCompletes(t *testing.T) {
	db := pgtest.Open(t)
	svc := &dcsToDcssrvc{DB: db}

	svc.auditReceivedExchange(context.Background(), "post_pdf", "did:web:dcs.example:contract:x", "did:web:peer.example", nil)
	svc.auditReceivedExchange(context.Background(), "post_pdf", "did:web:dcs.example:contract:x", "did:web:intruder.example", errors.New("refused"))

	events := archiveEvents(t, db, "PEER_EXCHANGE")
	require.Len(t, events, 1)
	require.Equal(t, "post_pdf", events[0]["exchange"])
	require.Equal(t, "RECEIVED", events[0]["direction"])
	require.Equal(t, "did:web:peer.example", events[0]["peer_did"])
}

// Every peer endpoint of the DcsToDcs service authenticates the peer and runs
// the federation trust gate through admitPeer, before anything else, and
// audits the exchange it received. get_provenance is a JWT-secured read for
// local users, not a peer endpoint.
func TestEveryPeerEndpointAdmitsThePeerFirstAndAuditsTheExchange(t *testing.T) {
	bodies := map[string]*ast.BlockStmt{}
	files, err := filepath.Glob("dcs_to_dcs*.go")
	require.NoError(t, err)
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		require.NoError(t, err)
		for _, decl := range parsed.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil && fn.Body != nil {
				bodies[fn.Name.Name] = fn.Body
			}
		}
	}

	service := reflect.TypeFor[dcstodcs.Service]()
	for i := range service.NumMethod() {
		method := service.Method(i).Name
		if method == "GetProvenance" {
			continue
		}
		body := bodies[method]
		if body == nil {
			t.Errorf("no handler body found for %s", method)
			continue
		}
		admit := callSites(body, "admitPeer")
		if len(admit) != 1 {
			t.Errorf("%s calls admitPeer %d times, want once", method, len(admit))
			continue
		}
		if _, ok := body.List[0].(*ast.DeferStmt); !ok {
			t.Errorf("%s does not start with the deferred audit of the exchange (DCS-NFR-BR-08)", method)
		}
		if len(callSites(body, "auditReceivedExchange")) != 1 {
			t.Errorf("%s does not audit the exchange it received (DCS-NFR-BR-08)", method)
		}
		for _, read := range []string{"BeginTxx", "ReadDataByDID", "ExistsByDID", "Handle", "AuthorizeErase", "Shred", "VerifyContent"} {
			if sites := callSites(body, read); len(sites) > 0 && sites[0] < admit[0] {
				t.Errorf("%s calls %s before it admits the peer", method, read)
			}
		}
	}
}
