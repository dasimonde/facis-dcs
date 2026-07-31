package status

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
)

type Verifier struct {
	ReferenceExtractor ReferenceExtractor
	Fetcher            *fetch.Client
	Handlers           map[Mechanism]Handler
	Policy             Policy
}

func (v *Verifier) routeReference(ctx context.Context, ref Reference) (Reference, error) {
	client := v.Fetcher
	if client == nil {
		client = fetch.NewClient()
	}

	resp, err := FetchStatusList(ctx, client, ref.URI, FetchOptsForReference(ref))
	if err != nil {
		return Reference{}, ErrStatusRetrieval
	}

	routed := ref
	routed.Mechanism = SelectMechanismFromResponse(ref, resp)
	routed.Prefetched = &resp
	return routed, nil
}

func (v *Verifier) VerifyStatus(
	ctx context.Context,
	credential VerifiedCredential,
) (CredentialVerificationResult, error) {
	extract := v.ReferenceExtractor
	if extract == nil {
		return CredentialVerificationResult{}, fmt.Errorf("status verifier: reference extractor is required")
	}

	refs, err := extract(credential)
	if err != nil {
		return CredentialVerificationResult{}, fmt.Errorf("status reference extraction failed: %w", err)
	}

	if len(refs) == 0 {
		policy := v.Policy
		if policy == nil {
			policy = StrictPolicy{}
		}
		return policy.HandleMissingStatus(credential)
	}

	results := make([]Result, 0, len(refs))
	for _, ref := range refs {
		routed, err := v.routeReference(ctx, ref)
		if err != nil {
			return CredentialVerificationResult{}, fmt.Errorf("status verification failed for %s: %w", ref.URI, err)
		}

		handler, ok := v.Handlers[routed.Mechanism]
		if !ok {
			return CredentialVerificationResult{}, fmt.Errorf("unsupported status mechanism: %s", routed.Mechanism)
		}

		result, err := handler.Check(ctx, credential, routed)
		if err != nil {
			return CredentialVerificationResult{}, fmt.Errorf("status verification failed for %s: %w", ref.URI, err)
		}
		results = append(results, result)
	}

	policy := v.Policy
	if policy == nil {
		policy = StrictPolicy{}
	}
	return policy.Evaluate(credential, results)
}
