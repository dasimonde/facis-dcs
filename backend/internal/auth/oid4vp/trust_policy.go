package oid4vp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/open-policy-agent/opa/v1/rego"

	"digital-contracting-service/internal/base/identity"
)

// trustPolicySource is the default authorization policy. It is the rules that
// used to be `if` statements — which issuer may do what, on whose behalf —
// expressed as data a deployment can read, diff and test with `opa test` rather
// than infer from control flow.
//
// A deployment may replace it via OID4VP_TRUST_POLICY_PATH. Nothing
// cryptographic is delegated: chain validation, signature and holder binding,
// and revocation stay in Go. Only the authorization decision is policy.
//
//go:embed policy/trust.rego
var trustPolicySource string

// TrustPolicyAttestationKey is the name the authorization policy is attested and
// pinned under (DCS-NFR-SEC-04). The policy outranks the trust document — a rule
// granting everything overrides every entry in it — so attesting the document
// while leaving the policy unpinned would put the pinned file under the unpinned
// one.
const TrustPolicyAttestationKey = "oid4vp-trust-policy"

// TrustPolicyPath is the policy file this deployment runs, or "" for the
// embedded default. Exported so startup can attest and pin it.
func TrustPolicyPath() string { return strings.TrimSpace(os.Getenv("OID4VP_TRUST_POLICY_PATH")) }

// Each decision is its own prepared query. Querying the whole package computed
// the denial reasons — string formatting, per issuer — on every credential
// verification, where the caller wanted one boolean.
const (
	queryTrusted   = "data.dcs.trust.trusted"
	queryMayAttest = "data.dcs.trust.may_attest"
	queryReasons   = "data.dcs.trust.reasons"
)

// The policy module does not depend on any one configuration, so it is compiled
// once for the process.
//
// It is read once, at startup. Editing the file in a running pod therefore has
// no effect until restart — which is the intended behaviour for something the
// startup attestation has hashed and pinned, but it does mean an in-place edit
// is silently inert rather than picked up.
var (
	preparedTrust map[string]rego.PreparedEvalQuery
	trustOnce     sync.Once
	trustErr      error
)

func policyModule() (string, string, error) {
	path := TrustPolicyPath()
	if path == "" {
		return "policy/trust.rego", trustPolicySource, nil
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("read trust policy %s: %w", path, err)
	}
	return path, string(source), nil
}

// PrepareTrustPolicy compiles the authorization policy. LoadTrustConfig calls it
// so a policy that cannot be compiled stops the process at startup, where the
// error is visible. Left to first use, a fat-fingered path produced a service
// that came up healthy and then refused every login, peer credential and PID.
func PrepareTrustPolicy() error {
	trustOnce.Do(func() {
		name, source, err := policyModule()
		if err != nil {
			trustErr = err
			return
		}
		prepared := make(map[string]rego.PreparedEvalQuery, 3)
		for _, query := range []string{queryTrusted, queryMayAttest, queryReasons} {
			q, err := rego.New(rego.Query(query), rego.Module(name, source)).PrepareForEval(context.Background())
			if err != nil {
				trustErr = fmt.Errorf("prepare trust policy %s (%s): %w", name, query, err)
				return
			}
			prepared[query] = q
		}
		preparedTrust = prepared
	})
	return trustErr
}

// policyDocument renders the trust document as the plain JSON the policy reads.
// It travels as evaluation input rather than as bound data: bound data is a
// snapshot taken once, and a configuration changed afterwards would go on being
// judged against what it said then, with nothing reporting the difference.
func (c *TrustConfig) policyDocument() map[string]any {
	issuers := make(map[string]any, len(c.Issuers))
	for iss, entry := range c.Issuers {
		iss = canonicalIssuerKey(iss)
		purposes := make([]any, 0, len(entry.Purposes))
		for _, p := range entry.Purposes {
			purposes = append(purposes, string(p))
		}
		organizations := make([]any, 0, len(entry.Organizations))
		for _, org := range entry.Organizations {
			organizations = append(organizations, strings.TrimSpace(org))
		}
		issuers[iss] = map[string]any{
			"purposes":      purposes,
			"organizations": organizations,
			"mechanism":     string(entry.Mechanism),
		}
	}
	return map[string]any{"issuers": issuers, "peer_dynamic": c.PeerDynamic}
}

func (c *TrustConfig) evalInput(purpose Purpose, iss, org string) map[string]any {
	return map[string]any{
		"purpose":      string(purpose),
		"issuer":       canonicalIssuerKey(iss),
		"organization": strings.TrimSpace(org),
		"trust":        c.policyDocument(),
	}
}

// canonicalIssuerKey is the form an issuer identifier is looked up under.
//
// The policy matches an entry by exact key, so a did:web issuer re-spelled in
// another case would miss its own entry and fall through to the dynamic-peer
// rule — turning purposes:["login"] into peer, which the entry was written to
// withhold. Both sides of the lookup are canonicalised so that cannot happen,
// and so it cannot start happening the day some other did:web comparison is
// made case-insensitive.
func canonicalIssuerKey(iss string) string {
	iss = strings.TrimSpace(iss)
	if strings.HasPrefix(iss, "did:web:") {
		return identity.NormalizeDIDWeb(iss)
	}
	return iss
}

// evaluateBool answers one yes/no question.
//
// A policy that cannot be evaluated denies. Treating a broken policy as
// permissive would turn a configuration mistake into silent trust, which is the
// failure mode this whole document exists to prevent.
func (c *TrustConfig) evaluateBool(query string, purpose Purpose, iss, org string) bool {
	if c == nil || PrepareTrustPolicy() != nil {
		return false
	}
	results, err := preparedTrust[query].Eval(context.Background(), rego.EvalInput(c.evalInput(purpose, iss, org)))
	if err != nil || len(results) == 0 || len(results[0].Expressions) == 0 {
		return false
	}
	allowed, _ := results[0].Expressions[0].Value.(bool)
	return allowed
}

// DenialReasons explains why an issuer was refused, for the error a caller
// reports. A policy that only answers false is a policy nobody can operate, and
// the operator mistakes that most need explaining — a policy defining no rule
// at all, or one that errors — are exactly the ones that produce no reason of
// their own, so those are named here.
func (v *PurposeView) DenialReasons(iss, org string) []string {
	if v == nil || v.cfg == nil {
		return []string{"no trust configuration is loaded"}
	}
	return v.cfg.denialReasons(v.purpose, iss, org)
}

func (c *TrustConfig) denialReasons(purpose Purpose, iss, org string) []string {
	source := "the built-in policy"
	if path := TrustPolicyPath(); path != "" {
		source = fmt.Sprintf("the policy at %s", path)
	}

	if err := PrepareTrustPolicy(); err != nil {
		return []string{fmt.Sprintf("%s could not be loaded, so nothing is trusted: %v", source, err)}
	}

	results, err := preparedTrust[queryReasons].Eval(context.Background(), rego.EvalInput(c.evalInput(purpose, iss, org)))
	if err != nil {
		return []string{fmt.Sprintf("%s failed to evaluate, so nothing is trusted: %v", source, err)}
	}
	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return []string{fmt.Sprintf("%s produced no decision for issuer %q, so it is not trusted", source, iss)}
	}

	raw, err := json.Marshal(results[0].Expressions[0].Value)
	if err != nil {
		return []string{fmt.Sprintf("%s produced an unreadable decision: %v", source, err)}
	}
	var reasons []string
	if err := json.Unmarshal(raw, &reasons); err != nil || len(reasons) == 0 {
		return []string{fmt.Sprintf("%s refused issuer %q for %s without stating a reason", source, iss, purpose)}
	}
	return reasons
}
