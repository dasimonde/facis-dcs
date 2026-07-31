// Package manifest parses a C2PA JUMBF manifest store into a lightweight
// enumeration of its manifest chain (DCS-OR-C2PA-008).
//
// The manifest store bytes are produced by pdf-core (compiler.renderJUMBFSuperbox
// et al., pdf-core/compiler/compiler_c2pa.go) and returned verbatim by the
// DCS backend's C2PA endpoint. This parser only needs the structural layout
// pdf-core emits — it is deliberately a minimal BMFF/JUMBF reader, not a full
// C2PA implementation:
//
//	jumb("c2pa")                              <- manifest store root
//	  jumb(<manifest-label>)                  <- one per manifest in the chain
//	    jumb("c2pa.assertions")
//	      jumb("dcs.lifecycle")               <- DCS lifecycle assertion
//	        cbor(<lifecycle map>)
//	      ...
//	    jumb("c2pa.claim.v2")
//	    jumb("c2pa.signature")
package manifest

import (
	"encoding/binary"
	"fmt"
)

// ChainEntry is one manifest in the C2PA manifest chain.
type ChainEntry struct {
	// Label is the manifest's JUMBF label (its urn:c2pa:... identifier).
	Label string `json:"label"`
	// Lifecycle is the parsed dcs.lifecycle assertion (contract_id, status,
	// file_hash, effective_at, ...) when present in this manifest, else nil.
	Lifecycle map[string]string `json:"lifecycle,omitempty"`
}

type box struct {
	typ     string
	payload []byte
}

// parseBoxes reads sibling BMFF boxes from data.
func parseBoxes(data []byte) ([]box, error) {
	var boxes []box
	for pos := 0; pos+8 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		if size < 8 || pos+size > len(data) {
			return nil, fmt.Errorf("invalid BMFF box framing at offset %d", pos)
		}
		boxes = append(boxes, box{
			typ:     string(data[pos+4 : pos+8]),
			payload: data[pos+8 : pos+size],
		})
		pos += size
	}
	return boxes, nil
}

// jumbfChildren returns the label and content boxes of a JUMBF superbox. The
// superbox is a "jumb" box whose payload starts with a "jumd" description box
// (carrying the label) followed by the content boxes.
func jumbfChildren(superboxPayload []byte) (label string, children []box, err error) {
	children, err = parseBoxes(superboxPayload)
	if err != nil {
		return "", nil, err
	}
	if len(children) == 0 || children[0].typ != "jumd" {
		return "", nil, fmt.Errorf("JUMBF description box (jumd) missing")
	}
	label, err = jumdLabel(children[0].payload)
	if err != nil {
		return "", nil, err
	}
	return label, children[1:], nil
}

// jumdLabel extracts the null-terminated label from a JUMBF description box
// payload: 16-byte UUID + 1 toggle byte + label + 0x00.
func jumdLabel(jumd []byte) (string, error) {
	if len(jumd) < 17 {
		return "", fmt.Errorf("JUMBF description box too small")
	}
	rest := jumd[17:]
	for i, b := range rest {
		if b == 0x00 {
			return string(rest[:i]), nil
		}
	}
	return "", fmt.Errorf("JUMBF label terminator missing")
}

// findChildSuperbox returns the content boxes of the child JUMBF superbox with
// the given label, or ok=false if none matches.
func findChildSuperbox(children []box, label string) (grandchildren []box, ok bool) {
	for _, c := range children {
		if c.typ != "jumb" {
			continue
		}
		childLabel, grandchildren, err := jumbfChildren(c.payload)
		if err != nil {
			continue
		}
		if childLabel == label {
			return grandchildren, true
		}
	}
	return nil, false
}

// ParseChain parses a C2PA JUMBF manifest store into its manifest chain. Each
// entry carries its manifest label and, when present, its parsed dcs.lifecycle
// assertion.
func ParseChain(store []byte) ([]ChainEntry, error) {
	rootBoxes, err := parseBoxes(store)
	if err != nil {
		return nil, err
	}
	if len(rootBoxes) == 0 || rootBoxes[0].typ != "jumb" {
		return nil, fmt.Errorf("C2PA manifest store root JUMBF box not found")
	}
	_, manifests, err := jumbfChildren(rootBoxes[0].payload)
	if err != nil {
		return nil, fmt.Errorf("read manifest store children: %w", err)
	}

	var entries []ChainEntry
	for _, m := range manifests {
		if m.typ != "jumb" {
			continue
		}
		label, manifestChildren, err := jumbfChildren(m.payload)
		if err != nil {
			return nil, fmt.Errorf("read manifest superbox: %w", err)
		}
		entry := ChainEntry{Label: label}

		if assertions, ok := findChildSuperbox(manifestChildren, "c2pa.assertions"); ok {
			if lifecycle, ok := findChildSuperbox(assertions, "dcs.lifecycle"); ok {
				for _, b := range lifecycle {
					if b.typ == "cbor" {
						if fields, err := parseCBORTextMap(b.payload); err == nil {
							entry.Lifecycle = fields
						}
						break
					}
				}
			}
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no manifests found in C2PA manifest store")
	}
	return entries, nil
}
