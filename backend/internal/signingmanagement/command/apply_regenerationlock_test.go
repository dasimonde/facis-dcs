package command

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/lib/pq"
)

// A lock wait cut short by lock_timeout means the background regenerator still
// holds the contract. Reporting it as ErrRegenerationInFlight is what keeps the
// caller from reading a generic database failure into contention that resolves
// on its own — the condition the whole bounded wait exists to surface.
func TestRegenerationLockErrorNamesContention(t *testing.T) {
	err := regenerationLockError("did:contract:1", &pq.Error{Code: pqLockNotAvailable, Message: "canceling statement due to lock timeout"})

	if !errors.Is(err, ErrRegenerationInFlight) {
		t.Fatalf("a lock_timeout must report ErrRegenerationInFlight, got %v", err)
	}
	if !strings.Contains(err.Error(), "did:contract:1") {
		t.Fatalf("the contract must be named, got %v", err)
	}
}

// Any other failure of the lock statement is a database failure, not
// contention: reporting it as retry-later would tell the caller to wait out a
// condition that is not going to clear.
func TestRegenerationLockErrorKeepsOtherFailuresDistinct(t *testing.T) {
	for name, cause := range map[string]error{
		"other postgres error": &pq.Error{Code: "42P01", Message: "relation does not exist"},
		"transport failure":    errors.New("connection reset by peer"),
	} {
		t.Run(name, func(t *testing.T) {
			err := regenerationLockError("did:contract:1", cause)

			if errors.Is(err, ErrRegenerationInFlight) {
				t.Fatalf("%v must not be reported as regeneration contention", cause)
			}
			if !errors.Is(err, cause) {
				t.Fatalf("the cause must be preserved, got %v", err)
			}
		})
	}
}

// The regenerator decides whether to leave a contract alone under the
// per-contract advisory lock, and its answer depends on the signature the
// signing path writes. That argument only holds if the WRITER holds the lock
// too: otherwise the sweep picks up an APPROVED contract with no stored CID,
// the submit spends seconds in DSS validation, the regenerator takes the free
// lock and fresh-renders, and whichever UPDATE commits last owns pdf_ipfs_cid.
// The invariant is per transaction, not per function — finalize writes the row
// inside its caller's transaction — so this walks the package's call graph:
// every transaction that reaches a signature write must reach the lock.
func TestEveryTransactionThatRecordsASignatureTakesTheRegenerationLock(t *testing.T) {
	graph := packageCallGraph(t)

	for name, calls := range graph {
		if !calls["BeginTxx"] {
			continue
		}
		if !reaches(graph, name, "CreateSignature") && !reaches(graph, name, "SetSignedPDF") {
			continue
		}
		if !reaches(graph, name, "acquireRegenerationLock") {
			t.Errorf("%s opens the transaction that records a signature but never takes the per-contract regeneration lock", name)
		}
	}
}

// Taking the lock is only half the invariant; WHERE it is taken decides what
// the whole suite costs. Held from the top of SubmitSignature it spans the DSS
// round trips, and the background regenerator — which this contract's own
// prepare() has just triggered — blocks on it for that entire window, on the
// handler every other contract's regeneration is queued behind. That is a
// measured three-fold BDD slowdown, not a theoretical one. The lock belongs
// between the last read and the first write: after DSS has answered, before the
// ceremony is consumed. This pins both edges.
func TestSubmitSignatureTakesTheRegenerationLockBetweenValidationAndTheWrites(t *testing.T) {
	body := functionBody(t, "apply.go", "SubmitSignature")

	lock := lastCallPos(body, "acquireRegenerationLock")
	validate := lastCallPos(body, "ValidatePDF")
	consume := lastCallPos(body, "MarkCeremonyConsumed")
	finalize := lastCallPos(body, "finalize")

	for name, pos := range map[string]token.Pos{
		"acquireRegenerationLock": lock, "ValidatePDF": validate,
		"MarkCeremonyConsumed": consume, "finalize": finalize,
	} {
		if pos == token.NoPos {
			t.Fatalf("SubmitSignature no longer calls %s: this test no longer checks what it claims", name)
		}
	}
	if lock < validate {
		t.Error("the regeneration lock is taken before DSS validation: the regenerator waits out every submitted signature's validation window")
	}
	if lock > consume || lock > finalize {
		t.Error("the regeneration lock is taken after a write: the regenerator can read this transaction's pre-signature state and render over the signed PDF")
	}
}

// functionBody parses one file of this package and returns the body of the
// named top-level function.
func functionBody(t *testing.T, file, name string) *ast.BlockStmt {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name && fn.Body != nil {
			return fn.Body
		}
	}
	t.Fatalf("%s declares no %s", file, name)
	return nil
}

// lastCallPos returns the position of the last call to name in body, or
// token.NoPos when it is never called.
func lastCallPos(body *ast.BlockStmt, name string) token.Pos {
	pos := token.NoPos
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var called string
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			called = fun.Name
		case *ast.SelectorExpr:
			called = fun.Sel.Name
		}
		if called == name && call.Pos() > pos {
			pos = call.Pos()
		}
		return true
	})
	return pos
}

// packageCallGraph maps every function declared in this package to the names it
// calls, closures included.
func packageCallGraph(t *testing.T) map[string]map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}

	fset := token.NewFileSet()
	graph := map[string]map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			calls := map[string]bool{}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					calls[fun.Name] = true
				case *ast.SelectorExpr:
					calls[fun.Sel.Name] = true
				}
				return true
			})
			graph[fn.Name.Name] = calls
		}
	}
	if len(graph) == 0 {
		t.Fatal("the package parsed to no functions")
	}
	return graph
}

// reaches reports whether from calls target directly or through any chain of
// functions declared in this package.
func reaches(graph map[string]map[string]bool, from, target string) bool {
	seen := map[string]bool{}
	var walk func(string) bool
	walk = func(name string) bool {
		if seen[name] {
			return false
		}
		seen[name] = true
		for callee := range graph[name] {
			if callee == target || walk(callee) {
				return true
			}
		}
		return false
	}
	return walk(from)
}
