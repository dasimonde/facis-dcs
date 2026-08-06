package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"sort"
	"strings"
	"time"
)

func buildC2PAExclusions(streamStart, streamLen int) []c2paExclusion {
	if streamLen <= 0 {
		return nil
	}
	return []c2paExclusion{{Start: streamStart, Length: streamLen}}
}

func exclusionsEqual(a, b []c2paExclusion) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// findObjectStreamRange returns the offset and length of the stream data of
// objectID's genesis definition — the C2PA exclusion window, which must name
// the manifest's own bytes and no other object's.
func findObjectStreamRange(pdf []byte, objectID int) (int, int, bool) {
	start, end, ok := firstObjectStreamData(pdf, objectID)
	if !ok {
		return 0, 0, false
	}
	return start, end - start, true
}

// findLastObjectStreamRange is findObjectStreamRange for the definition a
// reader resolves, after an incremental update has superseded the manifest.
func findLastObjectStreamRange(pdf []byte, objectID int) (int, int, bool) {
	start, end, ok := lastObjectStreamData(pdf, objectID)
	if !ok {
		return 0, 0, false
	}
	return start, end - start, true
}

func sha256WithExclusions(data []byte, exclusions []c2paExclusion) [32]byte {
	if len(exclusions) == 0 {
		return sha256.Sum256(data)
	}

	ranges := make([]c2paExclusion, 0, len(exclusions))
	for _, exclusion := range exclusions {
		if exclusion.Length <= 0 {
			continue
		}
		ranges = append(ranges, exclusion)
	}
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].Start < ranges[j].Start
	})

	hasher := sha256.New()
	pos := 0
	for _, exclusion := range ranges {
		if exclusion.Start > len(data) {
			break
		}
		if pos < exclusion.Start {
			_, _ = hasher.Write(data[pos:exclusion.Start])
		}
		excludeEnd := exclusion.Start + exclusion.Length
		if excludeEnd > len(data) {
			excludeEnd = len(data)
		}
		if excludeEnd > pos {
			pos = excludeEnd
		}
	}
	if pos < len(data) {
		_, _ = hasher.Write(data[pos:])
	}

	var out [32]byte
	copy(out[:], hasher.Sum(nil))
	return out
}

func parseBMFFBoxes(data []byte) ([]bmffBox, error) {
	var boxes []bmffBox
	for pos := 0; pos+8 <= len(data); {
		size := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		if size < 8 || pos+size > len(data) {
			return nil, fmt.Errorf("invalid BMFF box framing")
		}
		payloadStart := pos + 8
		boxes = append(boxes, bmffBox{
			Type:    string(data[pos+4 : pos+8]),
			Payload: append([]byte(nil), data[payloadStart:pos+size]...),
			Raw:     append([]byte(nil), data[pos:pos+size]...),
		})
		pos += size
	}
	return boxes, nil
}

func extractTopLevelManifestBoxes(c2pa []byte) ([][]byte, error) {
	rootBoxes, err := parseBMFFBoxes(c2pa)
	if err != nil {
		return nil, err
	}
	if len(rootBoxes) == 0 || rootBoxes[0].Type != "jumb" {
		return nil, fmt.Errorf("C2PA root JUMBF box not found")
	}
	rootChildren, err := parseBMFFBoxes(rootBoxes[0].Payload)
	if err != nil {
		return nil, err
	}
	var manifests [][]byte
	for _, child := range rootChildren {
		if child.Type == "jumb" {
			manifests = append(manifests, child.Raw)
		}
	}
	return manifests, nil
}

func extractJUMBFLabel(jumbBox []byte) (string, error) {
	boxes, err := parseBMFFBoxes(jumbBox)
	if err != nil {
		return "", err
	}
	if len(boxes) != 1 || boxes[0].Type != "jumb" {
		return "", fmt.Errorf("JUMBF superbox expected")
	}
	children, err := parseBMFFBoxes(boxes[0].Payload)
	if err != nil {
		return "", err
	}
	if len(children) == 0 || children[0].Type != "jumd" {
		return "", fmt.Errorf("JUMBF description box missing")
	}
	jumd := children[0].Payload
	if len(jumd) < 17 {
		return "", fmt.Errorf("JUMBF description box too small")
	}
	labelBytes := jumd[17:]
	idx := bytes.IndexByte(labelBytes, 0x00)
	if idx < 0 {
		return "", fmt.Errorf("JUMBF label terminator missing")
	}
	return string(labelBytes[:idx]), nil
}

func extractLabeledChildJUMBFBox(parentJumbBox []byte, label string) ([]byte, error) {
	boxes, err := parseBMFFBoxes(parentJumbBox)
	if err != nil {
		return nil, err
	}
	if len(boxes) != 1 || boxes[0].Type != "jumb" {
		return nil, fmt.Errorf("JUMBF superbox expected")
	}
	children, err := parseBMFFBoxes(boxes[0].Payload)
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if child.Type != "jumb" {
			continue
		}
		childLabel, err := extractJUMBFLabel(child.Raw)
		if err == nil && childLabel == label {
			return child.Raw, nil
		}
	}
	return nil, fmt.Errorf("child JUMBF box %s not found", label)
}

// The dcs.lifecycle statuses pdf-core itself decides (DCS-OR-C2PA-003). Every
// other status a manifest carries is read off the lifecycle credential the
// update attaches (lifecycleStatusFromVC).
const (
	// lifecycleStatusDraft is what a freshly compiled document is: an unsigned
	// contract with no lifecycle event behind it yet.
	lifecycleStatusDraft = "draft"
	// lifecycleStatusAmended is what an update that records no lifecycle event
	// is: the document changed, the contract's state did not.
	lifecycleStatusAmended = "amended"
)

func renderC2PAManifestStore(ctx context.Context, contractID string, payloadHash string, hardBindingHash []byte, exclusions []c2paExclusion, compiledAt time.Time) ([]byte, error) {
	// Build a deterministic, syntactically valid JUMBF C2PA manifest-store:
	//   jumb(c2pa) -> jumb(c2ma)
	//                    |- jumb(c2pa.assertions)
	//                    |- jumb(c2pa.claim.v2)
	//                    |- jumb(c2pa.signature)

	hardBindingAssertionPayload := renderMinimalDataHashAssertionCBOR(hardBindingHash, exclusions)
	hardBindingAssertionBox := renderJUMBFSuperbox(cborUUID, 0x03, "c2pa.hash.data", [][]byte{renderBMFFBox("cbor", hardBindingAssertionPayload)})
	hardBindingAssertionHash := sha256.Sum256(hardBindingAssertionBox[8:])

	actionsAssertionPayload := renderMinimalActionsAssertionCBOR()
	actionsAssertionBox := renderJUMBFSuperbox(cborUUID, 0x03, "c2pa.actions.v2", [][]byte{renderBMFFBox("cbor", actionsAssertionPayload)})
	actionsAssertionHash := sha256.Sum256(actionsAssertionBox[8:])

	lifecycleAssertionPayload := renderLifecycleAssertionCBOR(contractID, payloadHash, lifecycleStatusDraft, "", lifecycleAuthorityFromContext(ctx), "", "", compiledAt)
	lifecycleAssertionBox := renderJUMBFSuperbox(cborUUID, 0x03, "dcs.lifecycle", [][]byte{renderBMFFBox("cbor", lifecycleAssertionPayload)})
	lifecycleAssertionHash := sha256.Sum256(lifecycleAssertionBox[8:])

	assertionStore := renderJUMBFSuperbox(c2paAsrtUUID, 0x03, "c2pa.assertions", [][]byte{hardBindingAssertionBox, actionsAssertionBox, lifecycleAssertionBox})

	claimPayload := renderMinimalClaimCBOR(
		payloadHash,
		hardBindingAssertionHash[:],
		"self#jumbf=c2pa.assertions/c2pa.hash.data",
		actionsAssertionHash[:],
		"self#jumbf=c2pa.assertions/c2pa.actions.v2",
		lifecycleAssertionHash[:],
		"self#jumbf=c2pa.assertions/dcs.lifecycle",
	)
	claimBox := renderJUMBFSuperbox(c2paClmUUID, 0x03, "c2pa.claim.v2", [][]byte{renderBMFFBox("cbor", claimPayload)})

	// Tag(18) COSE_Sign1 with protected headers containing:
	//   1 -> -7   (alg=ES256)
	//   33 -> [certDer]  (x5chain per RFC 9360)
	// plus empty unprotected map, detached payload and a real signature over claim bytes.
	protected, err := buildCoseProtectedHeadersWithX5Chain(ctx)
	if err != nil {
		return nil, err
	}
	sig, err := signClaimSigStructure(ctx, protected, claimPayload)
	if err != nil {
		return nil, err
	}
	signaturePayload := []byte{0xD2, 0x84}
	signaturePayload = append(signaturePayload, cborBytes(protected)...)
	signaturePayload = append(signaturePayload, 0xA0, 0xF6)
	signaturePayload = append(signaturePayload, cborBytes(sig)...)
	signatureBox := renderJUMBFSuperbox(c2paSigUUID, 0x03, "c2pa.signature", [][]byte{renderBMFFBox("cbor", signaturePayload)})

	manifestLabel := urnUUIDFromHash(payloadHash)
	manifestBox := renderJUMBFSuperbox(c2paManifUUID, 0x03, manifestLabel, [][]byte{assertionStore, claimBox, signatureBox})
	return renderJUMBFSuperbox(c2paStoreUUID, 0x03, "c2pa", [][]byte{manifestBox}), nil
}

func renderVerificationManifestStore(ctx context.Context, originalC2PA []byte, manifestLabel string, contractID string, payloadHash string, lifecycleStatus string, hardBindingHash []byte, exclusions []c2paExclusion, compiledAt time.Time, remoteManifestURL string) ([]byte, error) {
	manifestBoxes, err := extractTopLevelManifestBoxes(originalC2PA)
	if err != nil {
		return nil, err
	}
	if len(manifestBoxes) == 0 {
		return nil, fmt.Errorf("no manifests found in original C2PA store")
	}
	originalManifestBox := manifestBoxes[len(manifestBoxes)-1]
	originalManifestLabel, err := extractJUMBFLabel(originalManifestBox)
	if err != nil {
		return nil, err
	}
	originalSignatureBox, err := extractLabeledChildJUMBFBox(originalManifestBox, "c2pa.signature")
	if err != nil {
		return nil, err
	}
	originalManifestHash := sha256.Sum256(originalManifestBox[8:])
	originalSignatureHash := sha256.Sum256(originalSignatureBox[8:])

	updateManifestBox, err := renderVerificationUpdateManifest(ctx, manifestLabel, contractID, payloadHash, lifecycleStatus, originalManifestLabel, originalManifestHash[:], originalSignatureHash[:], hardBindingHash, exclusions, compiledAt, remoteManifestURL)
	if err != nil {
		return nil, err
	}
	children := make([][]byte, 0, len(manifestBoxes)+1)
	children = append(children, manifestBoxes...)
	children = append(children, updateManifestBox)
	return renderJUMBFSuperbox(c2paStoreUUID, 0x03, "c2pa", children), nil
}

func renderVerificationUpdateManifest(ctx context.Context, manifestLabel string, contractID string, payloadHash string, lifecycleStatus string, parentManifestLabel string, parentManifestHash []byte, _ []byte, hardBindingHash []byte, exclusions []c2paExclusion, compiledAt time.Time, remoteManifestURL string) ([]byte, error) {
	updateLabel := manifestLabel
	hardBindingLabel := "c2pa.hash.data"
	hardBindingPayload := renderMinimalDataHashAssertionCBOR(hardBindingHash, exclusions)
	hardBindingBox := renderJUMBFSuperbox(cborUUID, 0x03, hardBindingLabel, [][]byte{renderBMFFBox("cbor", hardBindingPayload)})
	hardBindingAssertionHash := sha256.Sum256(hardBindingBox[8:])
	hardBindingURI := absoluteAssertionURI(updateLabel, hardBindingLabel)

	ingredientLabel := "c2pa.ingredient.v2"
	ingredientPayload := renderMinimalIngredientAssertionCBOR(payloadHash, parentManifestLabel, parentManifestHash)
	ingredientBox := renderJUMBFSuperbox(cborUUID, 0x03, ingredientLabel, [][]byte{renderBMFFBox("cbor", ingredientPayload)})
	ingredientHash := sha256.Sum256(ingredientBox[8:])
	ingredientURI := absoluteAssertionURI(updateLabel, ingredientLabel)

	actionsPayload := renderVerificationActionsAssertionCBOR(ingredientURI, ingredientHash[:])
	actionsBox := renderJUMBFSuperbox(cborUUID, 0x03, "c2pa.actions.v2", [][]byte{renderBMFFBox("cbor", actionsPayload)})
	actionsHash := sha256.Sum256(actionsBox[8:])
	actionsURI := absoluteAssertionURI(updateLabel, "c2pa.actions.v2")

	var assertionChildren [][]byte
	assertionChildren = append(assertionChildren, hardBindingBox, ingredientBox, actionsBox)
	var lifecycleURI string
	var lifecycleHash [32]byte
	if contractID != "" {
		lifecyclePayload := renderLifecycleAssertionCBOR(contractID, payloadHash, lifecycleStatus, "", lifecycleAuthorityFromContext(ctx), "", hex.EncodeToString(parentManifestHash), compiledAt)
		lifecycleBox := renderJUMBFSuperbox(cborUUID, 0x03, "dcs.lifecycle", [][]byte{renderBMFFBox("cbor", lifecyclePayload)})
		lifecycleHash = sha256.Sum256(lifecycleBox[8:])
		lifecycleURI = absoluteAssertionURI(updateLabel, "dcs.lifecycle")
		assertionChildren = append(assertionChildren, lifecycleBox)
	}
	var remoteManifestsURI string
	var remoteManifestsHash [32]byte
	if remoteManifestURL != "" {
		remoteManifestsPayload := renderRemoteManifestsAssertionCBOR(remoteManifestURL)
		remoteManifestsBox := renderJUMBFSuperbox(cborUUID, 0x03, "dcs.remote_manifests", [][]byte{renderBMFFBox("cbor", remoteManifestsPayload)})
		remoteManifestsHash = sha256.Sum256(remoteManifestsBox[8:])
		remoteManifestsURI = absoluteAssertionURI(updateLabel, "dcs.remote_manifests")
		assertionChildren = append(assertionChildren, remoteManifestsBox)
	}
	assertionStore := renderJUMBFSuperbox(c2paAsrtUUID, 0x03, "c2pa.assertions", assertionChildren)
	claimPayload := renderVerificationClaimCBOR(payloadHash, updateLabel, hardBindingURI, hardBindingAssertionHash[:], ingredientURI, ingredientHash[:], actionsURI, actionsHash[:], lifecycleURI, lifecycleHash[:], remoteManifestsURI, remoteManifestsHash[:])
	claimBox := renderJUMBFSuperbox(c2paClmUUID, 0x03, "c2pa.claim.v2", [][]byte{renderBMFFBox("cbor", claimPayload)})

	protected, err := buildCoseProtectedHeadersWithX5Chain(ctx)
	if err != nil {
		return nil, err
	}
	sig, err := signClaimSigStructure(ctx, protected, claimPayload)
	if err != nil {
		return nil, err
	}
	signaturePayload := []byte{0xD2, 0x84}
	signaturePayload = append(signaturePayload, cborBytes(protected)...)
	signaturePayload = append(signaturePayload, 0xA0, 0xF6)
	signaturePayload = append(signaturePayload, cborBytes(sig)...)
	signatureBox := renderJUMBFSuperbox(c2paSigUUID, 0x03, "c2pa.signature", [][]byte{renderBMFFBox("cbor", signaturePayload)})
	// A standard manifest box (c2ma), never an update manifest (c2um): c2pa-rs
	// ignores an update manifest's own hard binding and checks its parent's
	// against the current file, which can never match once the amendment has
	// appended an incremental PDF update. See
	// TestAmendmentManifestIsNotAC2PAUpdateManifest.
	return renderJUMBFSuperbox(c2paManifUUID, 0x03, updateLabel, [][]byte{assertionStore, claimBox, signatureBox}), nil
}

func renderJUMBFSuperbox(uuidHex string, toggles byte, label string, children [][]byte) []byte {
	desc := renderJUMBFDescriptionBox(uuidHex, toggles, label)
	payload := make([]byte, 0, len(desc)+128)
	payload = append(payload, desc...)
	for _, child := range children {
		payload = append(payload, child...)
	}
	return renderBMFFBox("jumb", payload)
}

func renderBMFFBox(boxType string, payload []byte) []byte {
	box := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(box[0:4], uint32(len(box)))
	copy(box[4:8], []byte(boxType))
	copy(box[8:], payload)
	return box
}

func renderUUIDContentBox(uuidHex string, payload []byte) []byte {
	uuid := decodeUUIDHex(uuidHex)
	boxPayload := make([]byte, 0, 16+len(payload))
	boxPayload = append(boxPayload, uuid[:]...)
	boxPayload = append(boxPayload, payload...)
	return renderBMFFBox("uuid", boxPayload)
}

func renderJUMBFDescriptionBox(uuidHex string, toggles byte, label string) []byte {
	uuid := decodeUUIDHex(uuidHex)
	payload := make([]byte, 0, 16+1+len(label)+1)
	payload = append(payload, uuid[:]...)
	payload = append(payload, toggles)
	if toggles&0x03 == 0x03 {
		payload = append(payload, []byte(label)...)
		payload = append(payload, 0x00)
	}
	return renderBMFFBox("jumd", payload)
}

func cborHead(major byte, n int) []byte {
	if n < 0 {
		n = 0
	}
	if n <= 23 {
		return []byte{byte((major << 5) | byte(n))}
	}
	if n <= 0xFF {
		return []byte{byte((major << 5) | 24), byte(n)}
	}
	if n <= 0xFFFF {
		return []byte{byte((major << 5) | 25), byte(n >> 8), byte(n)}
	}
	if n <= 0xFFFFFFFF {
		return []byte{byte((major << 5) | 26), byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	}
	u := uint64(n)
	return []byte{byte((major << 5) | 27), byte(u >> 56), byte(u >> 48), byte(u >> 40), byte(u >> 32), byte(u >> 24), byte(u >> 16), byte(u >> 8), byte(u)}
}

func cborText(s string) []byte {
	h := cborHead(3, len(s))
	return append(h, []byte(s)...)
}

func cborBytes(b []byte) []byte {
	h := cborHead(2, len(b))
	return append(h, b...)
}

func cborArray(items ...[]byte) []byte {
	out := cborHead(4, len(items))
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

func cborMap(pairs ...[]byte) []byte {
	out := cborHead(5, len(pairs)/2)
	for _, p := range pairs {
		out = append(out, p...)
	}
	return out
}

func cborUint(n int) []byte {
	return cborHead(0, n)
}

func cborNegInt(v int) []byte {
	// CBOR negative integer encoding stores -(n+1).
	return cborHead(1, -1-v)
}

func renderMinimalActionsAssertionCBOR() []byte {
	action := cborMap(
		cborText("action"), cborText("c2pa.created"),
		cborText("softwareAgent"), cborMap(
			cborText("name"), cborText("DCS-PDF-CORE"),
			cborText("version"), cborText("1.0"),
		),
	)
	return cborMap(
		cborText("actions"), cborArray(action),
		cborText("allActionsIncluded"), []byte{0xF5},
	)
}

func renderMinimalDataHashAssertionCBOR(hashBytes []byte, exclusions []c2paExclusion) []byte {
	exclusionItems := make([][]byte, 0, len(exclusions))
	for _, exclusion := range exclusions {
		if exclusion.Length <= 0 {
			continue
		}
		exclusionItems = append(exclusionItems, cborMap(
			cborText("start"), cborUint(exclusion.Start),
			cborText("length"), cborUint(exclusion.Length),
		))
	}

	pairs := [][]byte{
		cborText("alg"), cborText("sha256"),
		cborText("name"), cborText("pdf"),
		cborText("exclusions"), cborArray(exclusionItems...),
		cborText("hash"), cborBytes(hashBytes),
		cborText("pad"), cborBytes([]byte{0x00}),
	}
	return cborMap(pairs...)
}

// renderLifecycleAssertionCBOR renders the dcs.lifecycle assertion
// (DCS-OR-C2PA-003). fileHash is the SHA-256 of the contract.jsonld this
// manifest's document embeds — not of the PDF, which an assertion living inside
// that PDF cannot hash; c2pa.hash.data binds the file itself. The lifecycle VC's
// credentialSubject.file_hash is the artifact hash and is deliberately a
// different value (see provenance.LifecycleAssertion.FileHash).
func renderLifecycleAssertionCBOR(contractID, fileHash, status, reason, authority, vcID, prevManifestHash string, compiledAt time.Time) []byte {
	return cborMap(
		cborText("contract_id"), cborText(contractID),
		cborText("file_hash"), cborText(fileHash),
		cborText("status"), cborText(status),
		cborText("reason"), cborText(reason),
		cborText("effective_at"), cborText(compiledAt.UTC().Format(time.RFC3339)),
		cborText("authority"), cborText(authority),
		cborText("vc_id"), cborText(vcID),
		cborText("prev_manifest_hash"), cborText(prevManifestHash),
	)
}

func renderMinimalClaimCBOR(payloadHash string, hardBindingHash []byte, hardBindingURL string, actionsHash []byte, actionsURL string, lifecycleHash []byte, lifecycleURL string) []byte {
	instanceID := "xmp:iid:" + uuidFromHashPrefix(payloadHash)
	hardBindingHashedURI := cborMap(
		cborText("url"), cborText(hardBindingURL),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(hardBindingHash),
	)
	actionsHashedURI := cborMap(
		cborText("url"), cborText(actionsURL),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(actionsHash),
	)
	lifecycleHashedURI := cborMap(
		cborText("url"), cborText(lifecycleURL),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(lifecycleHash),
	)
	claimGenInfo := cborMap(
		cborText("name"), cborText("DCS-PDF-CORE"),
		cborText("version"), cborText("1.0"),
	)
	return cborMap(
		cborText("instanceID"), cborText(instanceID),
		cborText("claim_generator_info"), claimGenInfo,
		cborText("alg"), cborText("sha256"),
		cborText("signature"), cborText("self#jumbf=c2pa.signature"),
		cborText("created_assertions"), cborArray(hardBindingHashedURI, actionsHashedURI, lifecycleHashedURI),
	)
}

func renderMinimalIngredientAssertionCBOR(payloadHash string, parentManifestLabel string, parentManifestHash []byte) []byte {
	instanceID := "xmp:iid:" + uuidFromHashPrefix(payloadHash)
	return cborMap(
		cborText("dc:title"), cborText("DCS-PDF-CORE verified source"),
		cborText("dc:format"), cborText("application/pdf"),
		cborText("instanceID"), cborText(instanceID),
		cborText("relationship"), cborText("parentOf"),
		cborText("c2pa_manifest"), cborMap(
			cborText("url"), cborText(absoluteManifestURI(parentManifestLabel)),
			cborText("alg"), cborText("sha256"),
			cborText("hash"), cborBytes(parentManifestHash),
		),
	)
}

func renderVerificationActionsAssertionCBOR(ingredientURI string, ingredientHash []byte) []byte {
	ingredientRef := cborMap(
		cborText("url"), cborText(ingredientURI),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(ingredientHash),
	)
	opened := cborMap(
		cborText("action"), cborText("c2pa.opened"),
		cborText("parameters"), cborMap(
			cborText("ingredients"), cborArray(ingredientRef),
		),
		cborText("softwareAgent"), cborMap(
			cborText("name"), cborText("DCS-PDF-CORE"),
			cborText("version"), cborText("1.0"),
		),
	)
	return cborMap(
		cborText("actions"), cborArray(opened),
		cborText("allActionsIncluded"), []byte{0xF5},
	)
}

func renderVerificationClaimCBOR(payloadHash string, manifestLabel string, hardBindingURI string, hardBindingHash []byte, ingredientURI string, ingredientHash []byte, actionsURI string, actionsHash []byte, lifecycleURI string, lifecycleHash []byte, remoteManifestsURI string, remoteManifestsHash []byte) []byte {
	instanceID := "xmp:iid:" + uuidFromHashPrefix(payloadHash)
	hardBindingRef := cborMap(
		cborText("url"), cborText(hardBindingURI),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(hardBindingHash),
	)
	ingredientRef := cborMap(
		cborText("url"), cborText(ingredientURI),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(ingredientHash),
	)
	actionsRef := cborMap(
		cborText("url"), cborText(actionsURI),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(actionsHash),
	)
	lifecycleRef := cborMap(
		cborText("url"), cborText(lifecycleURI),
		cborText("alg"), cborText("sha256"),
		cborText("hash"), cborBytes(lifecycleHash),
	)
	claimGenInfo := cborMap(
		cborText("name"), cborText("DCS-PDF-CORE"),
		cborText("version"), cborText("1.0"),
	)
	// Remote-manifest discovery (DCS-OR-C2PA-008 AC3) is NOT expressed here as a
	// standalone "remote_manifests" claim field: c2pa-rs 0.85.1 / c2patool
	// 0.26.61 reject an unrecognized "remote_manifests" V2 claim field outright
	// ("claim could not be converted from CBOR"), breaking every DCS PDF that
	// ever carried one. Instead, when a manifest URL is supplied, a normal
	// named assertion — "dcs.remote_manifests", holding {"remote_manifests":
	// [url]} — is added to the assertion store (see
	// renderRemoteManifestsAssertionCBOR) and referenced here as a
	// hashed-URI in created_assertions, exactly like any other assertion
	// (mirroring dcs.lifecycle). The C2PA-normative discovery mechanism — a
	// dcterms:provenance link in the XMP metadata — is emitted in addition,
	// by renderXMPMetadata via the update appendix builders in update.go.
	// See pdf-core/features/manifest_url.feature.
	createdAssertions := [][]byte{hardBindingRef, ingredientRef, actionsRef, lifecycleRef}
	if remoteManifestsURI != "" {
		remoteManifestsRef := cborMap(
			cborText("url"), cborText(remoteManifestsURI),
			cborText("alg"), cborText("sha256"),
			cborText("hash"), cborBytes(remoteManifestsHash),
		)
		createdAssertions = append(createdAssertions, remoteManifestsRef)
	}
	pairs := [][]byte{
		cborText("instanceID"), cborText(instanceID),
		cborText("claim_generator_info"), claimGenInfo,
		cborText("alg"), cborText("sha256"),
		cborText("signature"), cborText(absoluteSignatureURI(manifestLabel)),
		cborText("created_assertions"), cborArray(createdAssertions...),
	}
	return cborMap(pairs...)
}

// renderRemoteManifestsAssertionCBOR builds the dcs.remote_manifests named
// assertion payload: {"remote_manifests": [url]}. This is a normal C2PA
// assertion (mirroring renderLifecycleAssertionCBOR), NOT a claim field —
// see the comment in renderVerificationClaimCBOR for why the two must not
// be conflated.
func renderRemoteManifestsAssertionCBOR(url string) []byte {
	return cborMap(
		cborText("remote_manifests"), cborArray(cborText(url)),
	)
}

// buildCoseProtectedHeadersWithX5Chain renders the COSE protected headers for
// one claim signature. The x5chain comes from the context and nowhere else:
// pdf-core holds no signing material, so a render that reaches a signature
// without one names no signer and is refused rather than completed under a
// default identity.
func buildCoseProtectedHeadersWithX5Chain(ctx context.Context) ([]byte, error) {
	chain, ok := signingChainFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("no C2PA signing chain in context: the caller must supply the x5chain it signs under")
	}
	chainItems := make([][]byte, 0, len(chain))
	for _, certDER := range chain {
		chainItems = append(chainItems, cborBytes(certDER))
	}

	return cborMap(
		cborUint(1), cborNegInt(-7),
		cborUint(33), cborArray(chainItems...),
	), nil
}

// coseDetachedSig64Marker is the CBOR framing that immediately precedes the
// 64-byte ES256 signature inside every COSE_Sign1 the compiler emits:
//
//	0xA0  empty map  (unprotected headers)
//	0xF6  null       (detached payload)
//	0x58 0x40  byte string of length 64  (the signature)
//
// The signature bytes themselves are the only non-deterministic part of a
// compiled PDF: ES256 over the HSM key is randomized, so two signings of the
// same claim differ. The determinism guarantee pdf-core relies on covers the
// human-readable content (fully determined by the JSON-LD), not the signature,
// which is verified separately against its x5chain. ZeroCOSESignatures masks
// those 64-byte runs so the deterministic re-render comparison ignores them.
var coseDetachedSig64Marker = []byte{0xA0, 0xF6, 0x58, 0x40}

// ZeroCOSESignatures returns a copy of pdf with every COSE_Sign1 ES256 signature
// (the 64 bytes following coseDetachedSig64Marker) zeroed, so byte comparisons
// of deterministically re-rendered PDFs are stable across randomized signings.
func ZeroCOSESignatures(pdf []byte) []byte {
	out := append([]byte(nil), pdf...)
	from := 0
	for {
		idx := bytes.Index(out[from:], coseDetachedSig64Marker)
		if idx < 0 {
			break
		}
		sigStart := from + idx + len(coseDetachedSig64Marker)
		sigEnd := sigStart + 64
		if sigEnd > len(out) {
			break
		}
		for i := sigStart; i < sigEnd; i++ {
			out[i] = 0
		}
		from = sigEnd
	}
	return out
}

// InjectCOSESignatures splices signatures into the zeroed 64-byte COSE_Sign1
// slots a CapturingSigner left behind, in document order — the stateless "embed"
// step. Each element of signatures must be a 64-byte ES256 r||s. Only all-zero
// slots are filled, so an update PDF's pre-existing real signatures are left
// untouched; the number of zeroed slots must equal len(signatures). No recompile
// happens, so embed depends on nothing but its inputs and any replica can serve
// it.
func InjectCOSESignatures(pdf []byte, signatures [][]byte) ([]byte, error) {
	for i, sig := range signatures {
		if len(sig) != 64 {
			return nil, fmt.Errorf("signature %d is %d bytes, want 64 (ES256 r||s)", i, len(sig))
		}
	}
	out := append([]byte(nil), pdf...)
	next := 0
	from := 0
	for {
		idx := bytes.Index(out[from:], coseDetachedSig64Marker)
		if idx < 0 {
			break
		}
		sigStart := from + idx + len(coseDetachedSig64Marker)
		sigEnd := sigStart + 64
		if sigEnd > len(out) {
			break
		}
		if isAllZero(out[sigStart:sigEnd]) {
			if next >= len(signatures) {
				return nil, fmt.Errorf("prepared PDF has more empty signature slots than the %d signatures provided", len(signatures))
			}
			copy(out[sigStart:sigEnd], signatures[next])
			next++
		}
		from = sigEnd
	}
	if next != len(signatures) {
		return nil, fmt.Errorf("prepared PDF has %d empty signature slots but %d signatures were provided", next, len(signatures))
	}
	return out, nil
}

// CountZeroedCOSESignatureSlots returns how many empty (all-zero) 64-byte
// COSE_Sign1 signature slots the PDF carries — the slots a CapturingSigner left
// for the embed step to fill. A render stabilises its byte layout over several
// passes, signing each pass but keeping only the last; this counts the slots that
// actually survived, so prepare returns exactly the final pass's Sig_structures.
func CountZeroedCOSESignatureSlots(pdf []byte) int {
	count := 0
	from := 0
	for {
		idx := bytes.Index(pdf[from:], coseDetachedSig64Marker)
		if idx < 0 {
			break
		}
		sigStart := from + idx + len(coseDetachedSig64Marker)
		sigEnd := sigStart + 64
		if sigEnd > len(pdf) {
			break
		}
		if isAllZero(pdf[sigStart:sigEnd]) {
			count++
		}
		from = sigEnd
	}
	return count
}

func isAllZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return len(b) > 0
}

// ParseSigningChainPEM decodes a PEM certificate chain into DER, leaf first, and
// enforces the C2PA certificate profile pdf-core depends on. Callers hand the
// result to WithSigningChain: pdf-core keeps no chain of its own, so every render
// that emits a manifest is signed under an identity its caller named.
func ParseSigningChainPEM(pemBytes []byte) ([][]byte, error) {
	certs, err := parseCertificateChainPEM(pemBytes)
	if err != nil {
		return nil, err
	}
	if err := requireLeafOrganization(certs[0]); err != nil {
		return nil, err
	}
	return certs, nil
}

func requireLeafOrganization(leafDER []byte) error {
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return fmt.Errorf("parse x5chain leaf certificate: %w", err)
	}
	if len(leaf.Subject.Organization) == 0 {
		return fmt.Errorf("x5chain leaf %q carries no organizationName; C2PA verifiers reject claims signed by it", leaf.Subject.CommonName)
	}
	return nil
}

func parseCertificateChainPEM(pemBytes []byte) ([][]byte, error) {
	rest := pemBytes
	var certs [][]byte
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		certs = append(certs, append([]byte(nil), block.Bytes...))
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("x5chain PEM does not include any CERTIFICATE blocks")
	}
	return certs, nil
}

func urnUUIDFromHash(payloadHash string) string {
	if len(payloadHash) < 32 {
		return "urn:c2pa:00000000-0000-0000-0000-000000000000"
	}
	h := strings.ToUpper(payloadHash[:32])
	return fmt.Sprintf("urn:c2pa:%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

func uuidFromHashPrefix(payloadHash string) string {
	h := strings.ToUpper(payloadHash)
	if len(h) < 32 {
		h += strings.Repeat("0", 32-len(h))
	}
	h = h[:32]
	return fmt.Sprintf("%s-%s-%s-%s-%s", h[0:8], h[8:12], h[12:16], h[16:20], h[20:32])
}

func payloadHashBytes(payloadHash string) []byte {
	if len(payloadHash) < 64 {
		return make([]byte, 32)
	}
	decoded, err := hex.DecodeString(payloadHash[:64])
	if err != nil || len(decoded) != 32 {
		return make([]byte, 32)
	}
	return decoded
}

func decodeUUIDHex(uuidHex string) [16]byte {
	var uuid [16]byte
	decoded, err := hex.DecodeString(uuidHex)
	if err != nil || len(decoded) != 16 {
		return uuid
	}
	copy(uuid[:], decoded)
	return uuid
}

// updateManifestLabel returns the label for the manifest an amendment appends,
// derived from the hard binding hash (SHA-256 of the file bytes at amendment
// time) rather than the payload hash. Amending the same payload twice — the
// negotiation ping-pong amends once per exchange — would otherwise mint the same
// label twice, so the second amendment would name itself as its parent and
// c2patool would reject the document outright with "cyclic ingredient found in
// path". The file grows on every hop, so the hard binding hash never repeats.
func updateManifestLabel(hardBindingHash []byte) string {
	return urnUUIDFromHash(hex.EncodeToString(hardBindingHash)) + ":dcs-pdf-core:2_1"
}

// witnessManifestLabel returns a manifest label for the verification witness
// derived from the hard binding hash (SHA-256 of the file bytes at witness
// time).  Using the hard binding hash instead of the payload hash guarantees
// the label is distinct from the update manifest that has already been written
// for the same payload — avoiding a cyclic-ingredient report from c2patool.
func witnessManifestLabel(hardBindingHash []byte) string {
	return urnUUIDFromHash(hex.EncodeToString(hardBindingHash)) + ":dcs-pdf-core:witness_1"
}

func absoluteManifestURI(manifestLabel string) string {
	return "self#jumbf=/c2pa/" + manifestLabel
}

func absoluteAssertionURI(manifestLabel string, assertionLabel string) string {
	return absoluteManifestURI(manifestLabel) + "/c2pa.assertions/" + assertionLabel
}

func absoluteSignatureURI(manifestLabel string) string {
	return absoluteManifestURI(manifestLabel) + "/c2pa.signature"
}

// extractLifecycleFields decodes the dcs.lifecycle assertion in the manifest at
// manifestIdx (0-based) of c2paBytes into its flat field map.
func extractLifecycleFields(c2paBytes []byte, manifestIdx int) (map[string]string, error) {
	manifestBoxes, err := extractTopLevelManifestBoxes(c2paBytes)
	if err != nil {
		return nil, fmt.Errorf("extract manifests: %w", err)
	}
	if manifestIdx >= len(manifestBoxes) {
		return nil, fmt.Errorf("manifest index %d out of range (%d manifests)", manifestIdx, len(manifestBoxes))
	}
	assertionStore, err := extractLabeledChildJUMBFBox(manifestBoxes[manifestIdx], "c2pa.assertions")
	if err != nil {
		return nil, fmt.Errorf("find assertion store: %w", err)
	}
	lifecycleBox, err := extractLabeledChildJUMBFBox(assertionStore, "dcs.lifecycle")
	if err != nil {
		return nil, fmt.Errorf("find dcs.lifecycle: %w", err)
	}
	outerBoxes, err := parseBMFFBoxes(lifecycleBox)
	if err != nil || len(outerBoxes) == 0 {
		return nil, fmt.Errorf("parse lifecycle jumb: %w", err)
	}
	innerBoxes, err := parseBMFFBoxes(outerBoxes[0].Payload)
	if err != nil {
		return nil, fmt.Errorf("parse lifecycle children: %w", err)
	}
	var cborData []byte
	for _, b := range innerBoxes {
		if b.Type == "cbor" {
			cborData = b.Payload
			break
		}
	}
	if cborData == nil {
		return nil, fmt.Errorf("cbor box not found in dcs.lifecycle")
	}
	fields, err := parseCBORTextMap(cborData)
	if err != nil {
		return nil, fmt.Errorf("decode lifecycle CBOR: %w", err)
	}
	return fields, nil
}

// extractManifestX5Chain returns the DER certificate chain (leaf first) that the
// manifest at manifestIdx (0-based) of c2paBytes carries in its COSE_Sign1
// protected headers. A recompile must reproduce it from the document itself: the
// leaf belongs to the DCS instance that signed that manifest, which for a
// contract received from a federation peer is not the instance verifying it, and
// the chain is embedded in the signed bytes rather than derived from the payload.
func extractManifestX5Chain(c2paBytes []byte, manifestIdx int) ([][]byte, error) {
	manifestBoxes, err := extractTopLevelManifestBoxes(c2paBytes)
	if err != nil {
		return nil, fmt.Errorf("extract manifests: %w", err)
	}
	if manifestIdx >= len(manifestBoxes) {
		return nil, fmt.Errorf("manifest index %d out of range (%d manifests)", manifestIdx, len(manifestBoxes))
	}
	signatureBox, err := extractLabeledChildJUMBFBox(manifestBoxes[manifestIdx], "c2pa.signature")
	if err != nil {
		return nil, fmt.Errorf("find claim signature: %w", err)
	}
	coseSign1, err := jumbfCBORPayload(signatureBox)
	if err != nil {
		return nil, fmt.Errorf("read claim signature: %w", err)
	}
	protected, _, err := decodeCOSESign1(coseSign1)
	if err != nil {
		return nil, fmt.Errorf("decode COSE_Sign1: %w", err)
	}
	return coseX5Chain(protected)
}

// ExtractSigningChain returns the x5chain of a PDF's genesis manifest, for a
// verifier that must recompile the base under the leaf that signed it.
func ExtractSigningChain(pdf []byte) ([][]byte, error) {
	c2paBytes, err := extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
	if err != nil {
		return nil, fmt.Errorf("extract C2PA: %w", err)
	}
	return extractManifestX5Chain(c2paBytes, 0)
}

// ManifestSigner names the certificate one manifest's claim signature was made
// under. Replaying a manifest under the leaf it carries proves the bytes are
// self-consistent; it says nothing about whose leaf that is. Reporting the leaf
// lets a caller compare it against the identity it expected.
type ManifestSigner struct {
	Manifest string
	LeafDER  []byte
}

// ManifestSigners lists, in chain order, the leaf each of a PDF's manifests was
// signed under.
func ManifestSigners(pdf []byte) ([]ManifestSigner, error) {
	c2paBytes, err := extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
	if err != nil {
		return nil, fmt.Errorf("extract C2PA: %w", err)
	}
	manifestBoxes, err := extractTopLevelManifestBoxes(c2paBytes)
	if err != nil {
		return nil, fmt.Errorf("extract manifests: %w", err)
	}
	signers := make([]ManifestSigner, 0, len(manifestBoxes))
	for i, manifest := range manifestBoxes {
		label, err := extractJUMBFLabel(manifest)
		if err != nil {
			return nil, fmt.Errorf("manifest %d: %w", i, err)
		}
		chain, err := extractManifestX5Chain(c2paBytes, i)
		if err != nil {
			return nil, fmt.Errorf("manifest %s: %w", label, err)
		}
		signers = append(signers, ManifestSigner{Manifest: label, LeafDER: chain[0]})
	}
	return signers, nil
}

// extractLifecycleAuthority returns the DID of the instance that asserted the
// manifest at manifestIdx. A recompile must reproduce it from the document
// itself, because it is carried by the manifest rather than the payload.
func extractLifecycleAuthority(c2paBytes []byte, manifestIdx int) (string, error) {
	fields, err := extractLifecycleFields(c2paBytes, manifestIdx)
	if err != nil {
		return "", err
	}
	return fields["authority"], nil
}

// extractLifecycleEffectiveAt extracts the effective_at timestamp from the
// dcs.lifecycle assertion in the manifest at manifestIdx (0-based) of c2paBytes.
func extractLifecycleEffectiveAt(c2paBytes []byte, manifestIdx int) (time.Time, error) {
	fields, err := extractLifecycleFields(c2paBytes, manifestIdx)
	if err != nil {
		return time.Time{}, err
	}
	s, ok := fields["effective_at"]
	if !ok || s == "" {
		return time.Time{}, fmt.Errorf("effective_at missing from dcs.lifecycle")
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse effective_at %q: %w", s, err)
	}
	return t, nil
}

// extractRemoteManifestURLFromXMP returns the dcterms:provenance URL from the
// PDF's current XMP metadata object (obj 13), or "" when none is present
// (DCS-OR-C2PA-008 AC3). The deterministic verify re-render
// (VerifyIncrementalUpdate) uses this so the re-applied amendment reproduces
// an XMP packet carrying the same provenance link — otherwise the stored PDF
// (which embeds it) would fail the byte-for-byte determinism check. See
// renderXMPMetadata's doc comment for why this replaced a C2PA claim field.
func extractRemoteManifestURLFromXMP(pdf []byte) string {
	const metadataObjID = 13
	start, length, found := findLastObjectStreamRange(pdf, metadataObjID)
	if !found {
		return ""
	}
	xmp := pdf[start : start+length]
	marker := []byte(`dcterms:provenance="`)
	idx := bytes.Index(xmp, marker)
	if idx < 0 {
		return ""
	}
	rest := xmp[idx+len(marker):]
	end := bytes.IndexByte(rest, '"')
	if end < 0 {
		return ""
	}
	return string(rest[:end])
}

// parseCBORTextMap decodes a CBOR map whose keys and values are all text strings.
func parseCBORTextMap(data []byte) (map[string]string, error) {
	if len(data) < 1 {
		return nil, fmt.Errorf("empty CBOR")
	}
	if data[0]>>5 != 5 {
		return nil, fmt.Errorf("expected CBOR map (major type 5), got %d", data[0]>>5)
	}
	add := int(data[0] & 0x1F)
	var count, pos int
	switch {
	case add <= 23:
		count, pos = add, 1
	case add == 24:
		if len(data) < 2 {
			return nil, fmt.Errorf("truncated map header")
		}
		count, pos = int(data[1]), 2
	default:
		return nil, fmt.Errorf("unsupported map size encoding (additional=%d)", add)
	}
	result := make(map[string]string, count)
	for i := 0; i < count; i++ {
		key, n, err := decodeCBORText(data[pos:])
		if err != nil {
			return nil, fmt.Errorf("key %d: %w", i, err)
		}
		pos += n
		val, n, err := decodeCBORText(data[pos:])
		if err != nil {
			return nil, fmt.Errorf("val %d: %w", i, err)
		}
		pos += n
		result[key] = val
	}
	return result, nil
}

// decodeCBORText decodes a single CBOR text string, returning the string,
// bytes consumed, and any error.
func decodeCBORText(data []byte) (string, int, error) {
	if len(data) < 1 {
		return "", 0, fmt.Errorf("empty")
	}
	if data[0]>>5 != 3 {
		return "", 0, fmt.Errorf("expected CBOR text (major type 3), got %d", data[0]>>5)
	}
	add := int(data[0] & 0x1F)
	var length, hdr int
	switch {
	case add <= 23:
		length, hdr = add, 1
	case add == 24:
		if len(data) < 2 {
			return "", 0, fmt.Errorf("truncated")
		}
		length, hdr = int(data[1]), 2
	default:
		return "", 0, fmt.Errorf("unsupported text length encoding (additional=%d)", add)
	}
	end := hdr + length
	if len(data) < end {
		return "", 0, fmt.Errorf("truncated text: need %d, have %d", end, len(data))
	}
	return string(data[hdr:end]), end, nil
}
