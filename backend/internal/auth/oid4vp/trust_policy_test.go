package oid4vp

import (
	"context"
	"embed"
	"strings"
	"testing"

	"github.com/open-policy-agent/opa/v1/ast"
	"github.com/open-policy-agent/opa/v1/storage/inmem"
	"github.com/open-policy-agent/opa/v1/tester"
)

//go:embed policy/*.rego
var policyFiles embed.FS

// The policy has its own tests, written in Rego. Running them here as well as
// under `opa test` means they cannot rot quietly: a developer who never installs
// the opa binary still runs them on every `go test ./...`, and CI needs no new
// tool.
func TestTrustPolicyRegoTests(t *testing.T) {
	entries, err := policyFiles.ReadDir("policy")
	if err != nil {
		t.Fatalf("read policy directory: %v", err)
	}

	modules := map[string]*ast.Module{}
	for _, entry := range entries {
		source, err := policyFiles.ReadFile("policy/" + entry.Name())
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		module, err := ast.ParseModule(entry.Name(), string(source))
		if err != nil {
			t.Fatalf("parse %s: %v", entry.Name(), err)
		}
		modules[entry.Name()] = module
	}
	if len(modules) < 2 {
		t.Fatalf("expected the policy and its tests, found %d module(s)", len(modules))
	}

	results, err := tester.NewRunner().
		SetStore(inmem.New()).
		SetModules(modules).
		RunTests(context.Background(), nil)
	if err != nil {
		t.Fatalf("run policy tests: %v", err)
	}

	var ran int
	for result := range results {
		ran++
		if result.Fail || result.Error != nil {
			t.Errorf("%s/%s failed: %v", result.Package, result.Name, result.Error)
		}
	}
	if ran == 0 {
		t.Fatal("no policy tests ran")
	}
}

// A denial has to say why. The rules are data now, so an operator reading a
// refusal needs the policy's own account of it rather than a bare false.
func TestDenialReasonsExplainTheRefusal(t *testing.T) {
	cfg := &TrustConfig{
		VCTs: []string{"urn:dcs:poa:v1"},
		Issuers: map[string]TrustedIssuer{
			"did:web:a.example:issuer": {
				Purposes:      []Purpose{PurposeLogin},
				Organizations: []string{"did:web:a.example"},
				Mechanism:     MechanismDIDWeb,
			},
		},
	}

	reasons := cfg.For(PurposePeer).DenialReasons("did:web:a.example:issuer", "")
	if len(reasons) == 0 {
		t.Fatal("a withheld purpose was refused with no explanation")
	}
	if !strings.Contains(strings.Join(reasons, "; "), "not granted") {
		t.Errorf("the reason does not name the missing grant: %v", reasons)
	}

	unlisted := cfg.For(PurposeLogin).DenialReasons("did:web:elsewhere.example:issuer", "")
	if !strings.Contains(strings.Join(unlisted, "; "), "not listed") {
		t.Errorf("the reason does not say the issuer is unlisted: %v", unlisted)
	}
}

// The document travels with each evaluation, so a configuration changed after
// its first use is judged against what it says now — not against a snapshot
// taken at startup that nothing would report.
func TestPolicySeesConfigurationChanges(t *testing.T) {
	cfg := &TrustConfig{PeerDynamic: false}
	peer := cfg.For(PurposePeer)

	if peer.IssuerTrusted("did:web:b.example:issuer") {
		t.Fatal("dynamic peer trust must be opt-in")
	}
	cfg.PeerDynamic = true
	if !peer.IssuerTrusted("did:web:b.example:issuer") {
		t.Error("the policy is still judging against the document it saw first")
	}
}
