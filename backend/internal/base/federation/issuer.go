package federation

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// CredentialIssuer serves this instance's agreement credential, re-minting it
// before the one it holds runs out.
//
// A credential with a bounded validUntil cannot be built once at startup and
// served for the life of the process: a deployment that stays up longer than
// AgreementCredentialLifetime would publish an expired credential and every peer
// would then refuse to talk to it. Peers hold nothing and are told nothing —
// each fetches the credential fresh as part of the trust gate, so replacing it
// here is the whole of the refresh.
type CredentialIssuer struct {
	Signer    Signer
	IssuerDID string
	RulesURL  string
	// Now is the clock, injectable for tests.
	Now func() time.Time

	mu       sync.Mutex
	held     json.RawMessage
	heldFrom time.Time
}

// Current returns a credential with at least half its lifetime left, minting a
// new one when the held credential is past that point. The margin is what keeps
// a fetch from racing the expiry: the peer that reads it verifies it moments
// later, and against its own clock.
func (i *CredentialIssuer) Current(ctx context.Context) (json.RawMessage, error) {
	if i == nil {
		return nil, fmt.Errorf("federation: no agreement credential issuer configured")
	}
	now := time.Now().UTC()
	if i.Now != nil {
		now = i.Now().UTC()
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.held != nil && now.Sub(i.heldFrom) < AgreementCredentialLifetime/2 {
		return i.held, nil
	}

	credential, err := BuildAgreementCredential(ctx, i.Signer, i.IssuerDID, i.RulesURL, now)
	if err != nil {
		return nil, err
	}
	i.held, i.heldFrom = credential, now
	return credential, nil
}
