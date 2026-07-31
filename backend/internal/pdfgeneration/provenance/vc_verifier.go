package provenance

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/mr-tron/base58"
)

// VerifyDataIntegrityProof verifies a VC's ecdsa-rdfc-2019 Data Integrity
// proof (see HSMVCSigner.CreateCredential, which produces it) against the
// given public key: it recomputes the RDFC-1.0 canonicalization of both the
// proof options and the document-without-proof, hashes each with SHA-256,
// and verifies the ECDSA signature encoded in proof.proofValue over their
// concatenation.
func VerifyDataIntegrityProof(vc json.RawMessage, pub *ecdsa.PublicKey) error {
	var vcMap map[string]interface{}
	if err := json.Unmarshal(vc, &vcMap); err != nil {
		return fmt.Errorf("unmarshal VC: %w", err)
	}

	proof, ok := vcMap["proof"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("VC carries no embedded proof")
	}
	if t, _ := proof["type"].(string); t != "DataIntegrityProof" {
		return fmt.Errorf("expected proof type DataIntegrityProof, got %q", t)
	}
	if suite, _ := proof["cryptosuite"].(string); suite != "ecdsa-rdfc-2019" {
		return fmt.Errorf("expected cryptosuite ecdsa-rdfc-2019, got %q", suite)
	}
	proofValue, _ := proof["proofValue"].(string)
	if !strings.HasPrefix(proofValue, "z") {
		return fmt.Errorf("expected a multibase base58btc proofValue ('z' prefix), got %q", proofValue)
	}
	sig, err := base58.Decode(strings.TrimPrefix(proofValue, "z"))
	if err != nil {
		return fmt.Errorf("decode proofValue: %w", err)
	}
	if len(sig) != 64 {
		return fmt.Errorf("expected a 64-byte raw ECDSA signature (r||s), got %d bytes", len(sig))
	}

	proofOptions := map[string]interface{}{}
	for k, v := range proof {
		if k == "proofValue" {
			continue
		}
		proofOptions[k] = v
	}
	proofOptions["@context"] = []interface{}{dataIntegrityContext}

	docWithoutProof := map[string]interface{}{}
	for k, v := range vcMap {
		if k == "proof" {
			continue
		}
		docWithoutProof[k] = v
	}

	proofNQ, err := normalizeVCJSONLD(proofOptions)
	if err != nil {
		return fmt.Errorf("normalize proof options: %w", err)
	}
	docNQ, err := normalizeVCJSONLD(docWithoutProof)
	if err != nil {
		return fmt.Errorf("normalize VC document: %w", err)
	}

	proofHash := sha256.Sum256([]byte(proofNQ))
	docHash := sha256.Sum256([]byte(docNQ))
	hashData := append(append([]byte{}, proofHash[:]...), docHash[:]...)
	digest := sha256.Sum256(hashData)

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return fmt.Errorf("ecdsa-rdfc-2019 signature verification failed")
	}
	return nil
}
