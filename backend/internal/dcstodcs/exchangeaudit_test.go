package dcstodcs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/require"
)

// DCS-NFR-BR-08: every outbound peer exchange records its delivery in the
// contract's audit trail.
func TestEveryOutboundPeerExchangeIsAudited(t *testing.T) {
	for _, site := range []struct{ file, function, call string }{
		{"synchronizer.go", "deliverPdf", "RecordPeerExchange"},
		{"settlement.go", "deliverSettlement", "RecordPeerExchangeTx"},
		{"settlement.go", "deliverSettlementWithdrawal", "RecordPeerExchangeTx"},
		{"negotiation.go", "deliverNegotiationMessage", "RecordPeerExchangeTx"},
		{"eraser.go", "attemptPeerErase", "RecordPeerExchange"},
		{"terminationnotifier.go", "recordConfirmation", "RecordPeerExchangeTx"},
	} {
		if !calls(t, site.file, site.function, site.call) {
			t.Errorf("%s does not audit the exchange it delivered", site.function)
		}
	}
}

func calls(t *testing.T, file, function, callee string) bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	require.NoError(t, err)
	found := false
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != function || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == callee {
					found = true
				}
			}
			return true
		})
	}
	return found
}
