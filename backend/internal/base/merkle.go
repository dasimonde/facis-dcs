package base

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// RFC 6962 domain separation: leaves and internal nodes are hashed under
// different prefixes so no internal node can be passed off as a leaf.
var (
	merkleLeafPrefix = []byte{0x00}
	merkleNodePrefix = []byte{0x01}
)

// MerkleLeafHash is the hash of one anchored audit entry, taken over the exact
// bytes stored in IPFS so a verifier can refetch the entry and recompute it.
func MerkleLeafHash(data []byte) string {
	h := sha256.New()
	h.Write(merkleLeafPrefix)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// MerkleRoot reduces the ordered leaf hashes of one checkpoint to a single
// root. An odd node at a level is promoted unchanged, as in RFC 6962.
func MerkleRoot(leafHashes []string) (string, error) {
	if len(leafHashes) == 0 {
		return "", errors.New("merkle root over no leaves")
	}
	level := make([][]byte, 0, len(leafHashes))
	for _, leaf := range leafHashes {
		raw, err := hex.DecodeString(leaf)
		if err != nil {
			return "", fmt.Errorf("leaf hash %q: %w", leaf, err)
		}
		level = append(level, raw)
	}
	for len(level) > 1 {
		next := make([][]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, level[i])
				continue
			}
			next = append(next, merkleNode(level[i], level[i+1]))
		}
		level = next
	}
	return hex.EncodeToString(level[0]), nil
}

// MerkleInclusionProof returns the sibling hashes, bottom-up, that carry the
// leaf at index up to the root — the evidence an auditor needs to show one
// entry belongs to a timestamped checkpoint without holding the whole batch.
func MerkleInclusionProof(leafHashes []string, index int) ([]string, error) {
	if index < 0 || index >= len(leafHashes) {
		return nil, fmt.Errorf("leaf index %d out of range for %d leaves", index, len(leafHashes))
	}
	level := make([][]byte, 0, len(leafHashes))
	for _, leaf := range leafHashes {
		raw, err := hex.DecodeString(leaf)
		if err != nil {
			return nil, fmt.Errorf("leaf hash %q: %w", leaf, err)
		}
		level = append(level, raw)
	}

	proof := make([]string, 0)
	for len(level) > 1 {
		next := make([][]byte, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i+1 == len(level) {
				next = append(next, level[i])
				continue
			}
			if i == index || i+1 == index {
				sibling := level[i+1]
				if i+1 == index {
					sibling = level[i]
				}
				proof = append(proof, hex.EncodeToString(sibling))
			}
			next = append(next, merkleNode(level[i], level[i+1]))
		}
		index /= 2
		level = next
	}
	return proof, nil
}

// VerifyMerkleInclusion recomputes the root from a leaf and its proof. It needs
// the checkpoint's leaf count (recorded alongside the root) because a level
// with an odd number of nodes promotes its last node without a sibling, so the
// proof is shorter than the tree is deep and the levels cannot be inferred from
// the proof alone.
func VerifyMerkleInclusion(leafHash string, proof []string, index, leafCount int, root string) bool {
	current, err := hex.DecodeString(leafHash)
	if err != nil {
		return false
	}
	consumed := 0
	for size := leafCount; size > 1; size = (size + 1) / 2 {
		if index != size-1 || size%2 == 0 {
			if consumed >= len(proof) {
				return false
			}
			raw, err := hex.DecodeString(proof[consumed])
			if err != nil {
				return false
			}
			consumed++
			if index%2 == 0 {
				current = merkleNode(current, raw)
			} else {
				current = merkleNode(raw, current)
			}
		}
		index /= 2
	}
	if consumed != len(proof) {
		return false
	}
	want, err := hex.DecodeString(root)
	if err != nil {
		return false
	}
	return bytes.Equal(current, want)
}

func merkleNode(left, right []byte) []byte {
	h := sha256.New()
	h.Write(merkleNodePrefix)
	h.Write(left)
	h.Write(right)
	return h.Sum(nil)
}
