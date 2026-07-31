// Command hsmsign signs an arbitrary UTF-8 message with a named HSM key
// (SHA-256, ECDSA ASN.1 DER — matching identity.DIDDocument.Sign) and prints
// the base64-encoded signature to stdout. It exists purely as a BDD test-fixture
// helper: DID signing uses the PKCS#11 dcs-did key (ECDSA P-256,
// non-extractable private key), so steps/peer_trust/dcs_peer_trust_steps.py's
// self-peer-simulation cannot produce a verifiable did:web challenge
// signature on its own and must shell out to this program, which opens the
// same SoftHSM2 token the target instance uses
// (selected via the usual PKCS11_*/SOFTHSM2_CONF env vars).
package main

import (
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"os"

	"digital-contracting-service/internal/base/hsm"
)

func main() {
	label := flag.String("label", "", "HSM key label to sign with, e.g. dcs-did")
	message := flag.String("message", "", "UTF-8 message to sign (SHA-256 digest is signed)")
	flag.Parse()

	if *label == "" || *message == "" {
		fmt.Fprintln(os.Stderr, "hsmsign: -label and -message are required")
		os.Exit(1)
	}

	h, err := hsm.Open(hsm.ConfigFromEnv())
	if err != nil {
		fmt.Fprintf(os.Stderr, "hsmsign: open token: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = h.Close() }()

	signer, err := h.Signer(*label)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hsmsign: load key %q: %v\n", *label, err)
		os.Exit(1)
	}

	hash := sha256.Sum256([]byte(*message))
	sig, err := signer.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hsmsign: sign: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(base64.StdEncoding.EncodeToString(sig))
}
