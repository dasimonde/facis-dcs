package provenance

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/base/hsm"

	"github.com/mr-tron/base58"
	"github.com/piprate/json-gold/ld"
)

// dataIntegrityContext defines the DataIntegrityProof/cryptosuite/proofValue
// terms used by the ecdsa-rdfc-2019 proof.
const dataIntegrityContext = "https://w3id.org/security/data-integrity/v2"

// HSMVCSigner adds an ECDSA (ES256) Data Integrity proof to a VC, signed by a
// P-256 key held in the PKCS#11 token (DCS-IR-HI-01). The proof suite is
// DataIntegrityProof with cryptosuite ecdsa-rdfc-2019: the document and proof
// options are canonicalized with RDFC-1.0 (URDNA2015), each hashed with
// SHA-256, and the concatenation is signed.
type HSMVCSigner struct {
	signer   crypto.Signer
	keyLabel string
}

// NewHSMVCSigner builds a VC signer over the given HSM key. keyLabel is the
// fragment appended to the issuer DID to form the proof's verificationMethod.
func NewHSMVCSigner(signer crypto.Signer, keyLabel string) *HSMVCSigner {
	return &HSMVCSigner{signer: signer, keyLabel: keyLabel}
}

// CreateCredential returns the VC with an ecdsa-rdfc-2019 Data Integrity proof.
func (s *HSMVCSigner) CreateCredential(_ context.Context, unsignedVC json.RawMessage) (json.RawMessage, error) {
	if s == nil || s.signer == nil {
		return nil, fmt.Errorf("hsm vc signer is not configured")
	}

	var vcMap map[string]interface{}
	if err := json.Unmarshal(unsignedVC, &vcMap); err != nil {
		return nil, fmt.Errorf("unmarshal unsigned VC: %w", err)
	}

	verificationMethod, err := s.verificationMethod(vcMap)
	if err != nil {
		return nil, err
	}

	proof := map[string]interface{}{
		"@context":           []interface{}{dataIntegrityContext},
		"type":               "DataIntegrityProof",
		"cryptosuite":        "ecdsa-rdfc-2019",
		"created":            time.Now().UTC().Format(time.RFC3339),
		"proofPurpose":       "assertionMethod",
		"verificationMethod": verificationMethod,
	}

	proofNQ, err := normalizeVCJSONLD(proof)
	if err != nil {
		return nil, fmt.Errorf("normalize proof options: %w", err)
	}
	docNQ, err := normalizeVCJSONLD(vcMap)
	if err != nil {
		return nil, fmt.Errorf("normalize VC document: %w", err)
	}

	proofHash := sha256.Sum256([]byte(proofNQ))
	docHash := sha256.Sum256([]byte(docNQ))
	hashData := append(append([]byte{}, proofHash[:]...), docHash[:]...)

	sig, err := hsm.SignES256(s.signer, hashData)
	if err != nil {
		return nil, fmt.Errorf("sign VC proof: %w", err)
	}

	// Multibase base58btc ('z' prefix), the encoding ecdsa-rdfc-2019 uses for
	// proofValue.
	proof["proofValue"] = "z" + base58.Encode(sig)
	// Strip the proof-only @context before embedding; the proof terms are
	// resolved against the VC's own top-level @context.
	delete(proof, "@context")
	vcMap["proof"] = proof

	out, err := json.Marshal(vcMap)
	if err != nil {
		return nil, fmt.Errorf("marshal signed VC: %w", err)
	}
	return json.RawMessage(out), nil
}

func (s *HSMVCSigner) verificationMethod(vc map[string]interface{}) (string, error) {
	label := strings.TrimSpace(s.keyLabel)
	if label == "" {
		return "", fmt.Errorf("vc key label is required to derive verificationMethod")
	}
	issuer, ok := vc["issuer"].(string)
	if !ok || strings.TrimSpace(issuer) == "" {
		return "", fmt.Errorf("VC issuer is required to derive verificationMethod")
	}
	return strings.TrimSpace(issuer) + "#" + label, nil
}

func normalizeVCJSONLD(doc interface{}) (string, error) {
	proc := ld.NewJsonLdProcessor()
	opts := ld.NewJsonLdOptions("")
	opts.Algorithm = ld.AlgorithmURDNA2015
	opts.Format = "application/n-quads"
	opts.DocumentLoader = vcDocumentLoader()
	norm, err := proc.Normalize(doc, opts)
	if err != nil {
		return "", err
	}
	return norm.(string), nil
}
