package service

import (
	"context"
	"encoding/json"

	"digital-contracting-service/internal/base/identity"

	didservice "digital-contracting-service/gen/did_service"
)

// AgreementCredentialSource hands out an agreement credential that is currently
// in force (federation.CredentialIssuer), kept as a local interface so this
// package does not depend on federation.
type AgreementCredentialSource interface {
	Current(ctx context.Context) (json.RawMessage, error)
}

type DIDSrv struct {
	DIDocument          identity.DIDDocument
	AgreementCredential AgreementCredentialSource
	FederationRules     []byte
}

// NewDIDService takes the issuer of this instance's signed agreement credential
// (ADR-19) and the embedded federation rules document (federation.Rules()). The
// credential is asked for per request rather than passed as bytes because it
// carries a bounded validUntil and is re-minted before it lapses.
func NewDIDService(didDocument identity.DIDDocument, agreementCredential AgreementCredentialSource, federationRules []byte) (didservice.Service, error) {
	return &DIDSrv{
		DIDocument:          didDocument,
		AgreementCredential: agreementCredential,
		FederationRules:     federationRules,
	}, nil
}

func (s DIDSrv) GetServiceDID(ctx context.Context) (res any, err error) {
	return s.DIDocument.GetDIDContent(), nil
}

// GetAgreementCredential serves this instance's self-signed federation
// agreement credential (ADR-19).
func (s DIDSrv) GetAgreementCredential(ctx context.Context) (res any, err error) {
	credential, err := s.AgreementCredential.Current(ctx)
	if err != nil {
		return nil, didservice.MakeInternalError(err)
	}
	var content map[string]interface{}
	if err := json.Unmarshal(credential, &content); err != nil {
		return nil, didservice.MakeInternalError(err)
	}
	return content, nil
}

// GetFederationRules serves the federation rules document embedded in this
// instance's binary (ADR-19); its content hash is the value the agreement
// credential's termsOfUse.hash names.
func (s DIDSrv) GetFederationRules(ctx context.Context) (res []byte, err error) {
	return s.FederationRules, nil
}
