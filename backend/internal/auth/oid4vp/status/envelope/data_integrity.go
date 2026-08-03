package envelope

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/mr-tron/base58/base58"
	"github.com/piprate/json-gold/ld"
)

const (
	CryptosuiteECDSARDFC2019  = "ecdsa-rdfc-2019"
	CryptosuiteEdDSARDFC2022  = "eddsa-rdfc-2022"
	proofTypeDataIntegrity    = "DataIntegrityProof"
	defaultVerificationMethod = "did:web:dev.example:issuer:poa#key-1"
)

type DataIntegritySigner interface {
	Sign(data []byte) ([]byte, error)
	Cryptosuite() string
	VerificationMethod() string
}

type ECDSASigner struct {
	PrivateKey           *ecdsa.PrivateKey
	VerificationMethodID string
}

func (s ECDSASigner) Cryptosuite() string { return CryptosuiteECDSARDFC2019 }
func (s ECDSASigner) VerificationMethod() string {
	if strings.TrimSpace(s.VerificationMethodID) != "" {
		return s.VerificationMethodID
	}
	return defaultVerificationMethod
}
func (s ECDSASigner) Sign(data []byte) ([]byte, error) {
	r, sVal, err := ecdsa.Sign(rand.Reader, s.PrivateKey, data)
	if err != nil {
		return nil, err
	}
	return append(padBigInt(r, 32), padBigInt(sVal, 32)...), nil
}

type Ed25519Signer struct {
	PrivateKey           ed25519.PrivateKey
	VerificationMethodID string
}

func (s Ed25519Signer) Cryptosuite() string { return CryptosuiteEdDSARDFC2022 }
func (s Ed25519Signer) VerificationMethod() string {
	if strings.TrimSpace(s.VerificationMethodID) != "" {
		return s.VerificationMethodID
	}
	return defaultVerificationMethod
}
func (s Ed25519Signer) Sign(data []byte) ([]byte, error) {
	return ed25519.Sign(s.PrivateKey, data), nil
}

type DataIntegrityVerifier struct {
	ResolveECDSA   func(issuer string) (*ecdsa.PublicKey, error)
	ResolveEd25519 func(issuer string) (ed25519.PublicKey, error)
	DocumentLoader ld.DocumentLoader
}

func (v DataIntegrityVerifier) loader() ld.DocumentLoader {
	if v.DocumentLoader != nil {
		return v.DocumentLoader
	}
	return EmbeddedDocumentLoader()
}

func SignDataIntegrityCredential(document map[string]any, signer DataIntegritySigner, created string) (map[string]any, error) {
	if created == "" {
		created = "2024-06-23T00:00:00Z"
	}
	proof := map[string]any{
		"type":               proofTypeDataIntegrity,
		"cryptosuite":        signer.Cryptosuite(),
		"created":            created,
		"verificationMethod": signer.VerificationMethod(),
		"proofPurpose":       "assertionMethod",
	}
	verifyData, err := buildVerifyData(document, proof, DefaultDocumentLoader())
	if err != nil {
		return nil, err
	}
	signature, err := signer.Sign(verifyData)
	if err != nil {
		return nil, err
	}
	proof["proofValue"] = "z" + base58.Encode(signature)

	signed := cloneMap(document)
	signed["proof"] = proof
	return signed, nil
}

func VerifyDataIntegrityCredential(raw []byte, verifier DataIntegrityVerifier) (map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		return nil, fmt.Errorf("invalid json credential: %w", err)
	}
	proof, err := extractProof(document)
	if err != nil {
		return nil, err
	}
	cryptosuite, _ := proof["cryptosuite"].(string)
	verifyData, err := buildVerifyData(document, proof, verifier.loader())
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

	switch cryptosuite {
	case CryptosuiteECDSARDFC2019:
		if verifier.ResolveECDSA == nil {
			return nil, fmt.Errorf("ecdsa resolver is required")
		}
		pub, err := verifier.ResolveECDSA(issuer)
		if err != nil {
			return nil, err
		}
		if len(signature) != 64 {
			return nil, fmt.Errorf("invalid ecdsa signature length %d", len(signature))
		}
		r := new(big.Int).SetBytes(signature[:32])
		s := new(big.Int).SetBytes(signature[32:])
		if !ecdsa.Verify(pub, verifyData, r, s) {
			return nil, fmt.Errorf("data integrity proof verification failed")
		}
	case CryptosuiteEdDSARDFC2022:
		if verifier.ResolveEd25519 == nil {
			return nil, fmt.Errorf("ed25519 resolver is required")
		}
		pub, err := verifier.ResolveEd25519(issuer)
		if err != nil {
			return nil, err
		}
		if !ed25519.Verify(pub, verifyData, signature) {
			return nil, fmt.Errorf("data integrity proof verification failed")
		}
	default:
		return nil, fmt.Errorf("unsupported cryptosuite %q", cryptosuite)
	}

	return document, nil
}

func buildVerifyData(document map[string]any, proof map[string]any, loader ld.DocumentLoader) ([]byte, error) {
	docWithoutProof := cloneMap(document)
	delete(docWithoutProof, "proof")

	proofOptions := buildProofOptions(document, proof)
	proofHash, err := hashCanonized(proofOptions, loader)
	if err != nil {
		return nil, err
	}
	docHash, err := hashCanonized(docWithoutProof, loader)
	if err != nil {
		return nil, err
	}
	verifyData := make([]byte, len(proofHash)+len(docHash))
	copy(verifyData, proofHash)
	copy(verifyData[len(proofHash):], docHash)
	return verifyData, nil
}

func buildProofOptions(document map[string]any, proof map[string]any) map[string]any {
	proofOptions := cloneMap(proof)
	delete(proofOptions, "proofValue")

	contexts := collectContexts(document["@context"])
	contexts = appendContext(contexts, dataIntegrityV2ContextURL)
	proofOptions["@context"] = contexts
	return proofOptions
}

func hashCanonized(document any, loader ld.DocumentLoader) ([]byte, error) {
	nquads, err := canonizeRDF(document, loader)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(nquads))
	return sum[:], nil
}

func extractProof(document map[string]any) (map[string]any, error) {
	raw := document["proof"]
	switch proof := raw.(type) {
	case map[string]any:
		return proof, nil
	case []any:
		if len(proof) == 0 {
			return nil, fmt.Errorf("credential proof is empty")
		}
		first, ok := proof[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("credential proof has invalid shape")
		}
		return first, nil
	default:
		return nil, fmt.Errorf("credential is missing proof")
	}
}

func decodeMultibaseProofValue(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, fmt.Errorf("proofValue is required")
	}
	if value[0] != 'z' {
		return nil, fmt.Errorf("unsupported multibase encoding %q", value[:1])
	}
	return base58.Decode(value[1:])
}

func issuerFromVerificationMethod(verificationMethod string) string {
	verificationMethod = strings.TrimSpace(verificationMethod)
	if idx := strings.Index(verificationMethod, "#"); idx > 0 {
		return verificationMethod[:idx]
	}
	return verificationMethod
}

func collectContexts(raw any) []any {
	switch value := raw.(type) {
	case string:
		return []any{value}
	case []any:
		return append([]any(nil), value...)
	default:
		return []any{credentialsV2ContextURL}
	}
}

func appendContext(contexts []any, url string) []any {
	for _, item := range contexts {
		if s, ok := item.(string); ok && s == url {
			return contexts
		}
	}
	return append(contexts, url)
}

func cloneMap(in map[string]any) map[string]any {
	raw, _ := json.Marshal(in)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func padBigInt(v *big.Int, size int) []byte {
	out := make([]byte, size)
	b := v.Bytes()
	copy(out[size-len(b):], b)
	return out
}
