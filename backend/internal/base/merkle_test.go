package base

import (
	"fmt"
	"testing"
)

func leaves(n int) []string {
	hashes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		hashes = append(hashes, MerkleLeafHash([]byte(fmt.Sprintf("audit entry %d", i))))
	}
	return hashes
}

// Every leaf of a checkpoint must be provable against that checkpoint's root,
// at every batch size — including the odd sizes where a node is promoted.
func TestInclusionProofHoldsForEveryLeaf(t *testing.T) {
	for _, size := range []int{1, 2, 3, 5, 8, 17, 100} {
		hashes := leaves(size)
		root, err := MerkleRoot(hashes)
		if err != nil {
			t.Fatalf("size %d: %v", size, err)
		}
		for i := range hashes {
			proof, err := MerkleInclusionProof(hashes, i)
			if err != nil {
				t.Fatalf("size %d leaf %d: %v", size, i, err)
			}
			if !VerifyMerkleInclusion(hashes[i], proof, i, size, root) {
				t.Fatalf("size %d: leaf %d does not verify against the root", size, i)
			}
		}
	}
}

// Tamper evidence: changing one anchored entry must break its proof against
// the timestamped root.
func TestAlteredLeafFailsVerification(t *testing.T) {
	hashes := leaves(9)
	root, err := MerkleRoot(hashes)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := MerkleInclusionProof(hashes, 4)
	if err != nil {
		t.Fatal(err)
	}
	altered := MerkleLeafHash([]byte("audit entry 4, edited after the fact"))
	if VerifyMerkleInclusion(altered, proof, 4, len(hashes), root) {
		t.Fatal("an altered entry still verified against the checkpoint root")
	}
}

// A leaf must not be passable as an internal node (RFC 6962 domain separation).
func TestLeafAndNodeHashesAreDomainSeparated(t *testing.T) {
	data := []byte("audit entry")
	leaf := MerkleLeafHash(data)
	pair, err := MerkleRoot([]string{leaf, leaf})
	if err != nil {
		t.Fatal(err)
	}
	if pair == leaf {
		t.Fatal("internal node hash collides with its leaf hash")
	}
}

func TestRootIsOrderSensitive(t *testing.T) {
	hashes := leaves(4)
	forward, err := MerkleRoot(hashes)
	if err != nil {
		t.Fatal(err)
	}
	swapped := []string{hashes[1], hashes[0], hashes[2], hashes[3]}
	reordered, err := MerkleRoot(swapped)
	if err != nil {
		t.Fatal(err)
	}
	if forward == reordered {
		t.Fatal("reordering entries left the root unchanged; batch order is not committed to")
	}
}

func TestRootOverNoLeavesIsRefused(t *testing.T) {
	if _, err := MerkleRoot(nil); err == nil {
		t.Fatal("expected an error for a checkpoint with no leaves")
	}
}

// The blinding nonce is what makes a leaf hash publishable: two audit entries
// that differ only by their nonce must not produce the same leaf, otherwise a
// published proof would be an unsalted commitment over guessable content and
// anyone could brute-force it to confirm what an entry says.
func TestLeafHashIsBlindedByTheNonce(t *testing.T) {
	const entry = `{"id":42,"component":"ContractWorkflowEngine","event_type":"TERMINATE_CONTRACT",` +
		`"did":"did:example:contract:1","created_at":"2026-07-21T14:03:07Z","res_log_pred_cid":null,"nonce":%q}`

	first := MerkleLeafHash([]byte(fmt.Sprintf(entry, "8f14e45fceea167a5a36dedd4bea2543")))
	second := MerkleLeafHash([]byte(fmt.Sprintf(entry, "c9f0f895fb98ab9159f51fd0297e236d")))
	if first == second {
		t.Fatal("the nonce does not enter the leaf hash; a published leaf would be guessable")
	}
}
