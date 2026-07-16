package envelope

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/piprate/json-gold/ld"
)

const ProofTypeEd25519Signature2020 = "Ed25519Signature2020"

type Ed25519Signature2020Verifier struct {
	ResolveEd25519 func(issuer string) (ed25519.PublicKey, error)
	DocumentLoader ld.DocumentLoader
}

func (v Ed25519Signature2020Verifier) loader() ld.DocumentLoader {
	if v.DocumentLoader != nil {
		return v.DocumentLoader
	}
	return DefaultDocumentLoader()
}

// VerifyEd25519Signature2020Credential verifies a Verifiable Credential
// secured with an Ed25519Signature2020 Linked Data Proof.
func VerifyEd25519Signature2020Credential(raw []byte, verifier Ed25519Signature2020Verifier) (map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("invalid json credential: %w", err)
	}

	proof, err := extractProof(document)
	if err != nil {
		return nil, err
	}
	proofType, _ := proof["type"].(string)
	if strings.TrimSpace(proofType) != ProofTypeEd25519Signature2020 {
		return nil, fmt.Errorf("proof type must be %s", ProofTypeEd25519Signature2020)
	}

	verifyData, err := buildVerifyDataEd25519Signature2020(document, proof, verifier.loader())
	if err != nil {
		return nil, err
	}

	proofValue, _ := proof["proofValue"].(string)
	signature, err := decodeMultibaseProofValue(proofValue)
	if err != nil {
		return nil, err
	}

	verificationMethod, _ := proof["verificationMethod"].(string)
	issuer := issuerFromVerificationMethod(verificationMethod)
	if issuer == "" {
		if issuerClaim, ok := document["issuer"].(string); ok {
			issuer = strings.TrimSpace(issuerClaim)
		}
	}
	if issuer == "" {
		return nil, fmt.Errorf("unable to resolve issuer for Ed25519Signature2020 proof")
	}
	if verifier.ResolveEd25519 == nil {
		return nil, fmt.Errorf("ed25519 resolver is required")
	}
	pub, err := verifier.ResolveEd25519(issuer)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(pub, verifyData, signature) {
		return nil, fmt.Errorf("ed25519 signature2020 proof verification failed")
	}

	return document, nil
}

func buildVerifyDataEd25519Signature2020(document map[string]any, proof map[string]any, loader ld.DocumentLoader) ([]byte, error) {
	docWithoutProof := cloneMap(document)
	delete(docWithoutProof, "proof")

	proofOptions := cloneMap(proof)
	delete(proofOptions, "jws")
	delete(proofOptions, "signatureValue")
	delete(proofOptions, "proofValue")
	if contexts, ok := document["@context"]; ok {
		proofOptions["@context"] = contexts
	}

	proofNquads, err := canonizeRDF(proofOptions, loader)
	if err != nil {
		return nil, err
	}
	docNquads, err := canonizeRDF(docWithoutProof, loader)
	if err != nil {
		return nil, err
	}

	proofHash := sha256.Sum256([]byte(proofNquads))
	docHash := sha256.Sum256([]byte(docNquads))
	verifyData := make([]byte, len(proofHash)+len(docHash))
	copy(verifyData, proofHash[:])
	copy(verifyData[len(proofHash):], docHash[:])
	return verifyData, nil
}
