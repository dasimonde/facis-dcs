package bundleexport

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
)

func TestZipWithManifestHashesMatchPackagedBytes(t *testing.T) {
	files := bundleFiles{
		"contract.jsonld":                 []byte(`{"@id":"did:example:c"}`),
		"contract.pdf":                    []byte("%PDF-1.7 fake"),
		"manifest-store.c2pa":             []byte("c2pa-bytes"),
		"credentials/manifest-chain.json": []byte(`[]`),
		"signatures.json":                 []byte(`[]`),
	}
	components := []componentInfo{{DID: "did:example:c", State: "SIGNED", Role: "root"}}

	zipBytes, err := zipWithManifest(files, "contract", "did:example:c", components)
	if err != nil {
		t.Fatalf("zipWithManifest: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}

	contents := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		contents[f.Name] = data
	}

	// bundle-manifest.json must exist and be excluded from its own entries.
	manifestRaw, ok := contents["bundle-manifest.json"]
	if !ok {
		t.Fatalf("bundle-manifest.json missing from zip")
	}
	var manifest bundleManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Entries) != len(files) {
		t.Fatalf("expected %d entries, got %d", len(files), len(manifest.Entries))
	}

	for _, e := range manifest.Entries {
		if e.Path == "bundle-manifest.json" {
			t.Fatalf("manifest must not list itself")
		}
		packed, ok := contents[e.Path]
		if !ok {
			t.Fatalf("manifest entry %s not present in zip", e.Path)
		}
		sum := sha256.Sum256(packed)
		if hex.EncodeToString(sum[:]) != e.SHA256 {
			t.Fatalf("sha256 mismatch for %s", e.Path)
		}
	}

	// Required contract members are present.
	for _, want := range []string{"contract.jsonld", "contract.pdf", "manifest-store.c2pa", "signatures.json"} {
		if _, ok := contents[want]; !ok {
			t.Fatalf("missing required member %s", want)
		}
	}
	if _, ok := contents["credentials/manifest-chain.json"]; !ok {
		t.Fatalf("missing credentials/ member")
	}
}

// familyRepoFake resolves the hierarchy family from an in-memory
// did->parentDID map, mirroring command's cycleRepoFake.
type familyRepoFake struct {
	cwedb.ContractRepo
	parents   map[string]string // did -> parentDID ("" = no parent)
	known     map[string]bool   // dids that resolve locally
	createdBy map[string]string // did -> creating organization (optional)
}

func (r *familyRepoFake) ReadDataByDID(_ context.Context, _ *sqlx.Tx, did string) (*cwedb.Contract, error) {
	if !r.known[did] {
		return nil, fmt.Errorf("contract with DID %s not found", did)
	}
	c := &cwedb.Contract{DID: did, CreatedBy: r.createdBy[did]}
	if parent := r.parents[did]; parent != "" {
		data, err := datatype.NewJSON(map[string]any{
			"dcs:parentContract": map[string]any{"@id": parent},
		})
		if err != nil {
			return nil, err
		}
		c.ContractData = &data
	}
	return c, nil
}

func (r *familyRepoFake) ReadChildrenDIDs(_ context.Context, _ *sqlx.Tx, did string) ([]string, error) {
	var children []string
	for child, parent := range r.parents {
		if parent == did && r.known[child] {
			children = append(children, child)
		}
	}
	sort.Strings(children)
	return children, nil
}

func TestResolveFamilyIncludesLocalSiblings(t *testing.T) {
	// A is the frame; B and C are its children; D is B's child. Every start
	// point resolves the same family in the same deterministic order.
	repo := &familyRepoFake{
		parents: map[string]string{"B": "A", "C": "A", "D": "B"},
		known:   map[string]bool{"A": true, "B": true, "C": true, "D": true},
	}

	for _, start := range []string{"B", "A", "D"} {
		family, err := resolveFamily(context.Background(), nil, repo, start)
		if err != nil {
			t.Fatalf("resolve from %s: %v", start, err)
		}
		if !reflect.DeepEqual(family, []string{"A", "B", "C", "D"}) {
			t.Fatalf("resolve from %s: got %v", start, family)
		}
	}
}

func TestResolveFamilyToleratesNonLocalMembers(t *testing.T) {
	// X's parent lives on another instance: the walk up stops at X, which
	// becomes the family root. X's sibling (another child of the remote
	// frame) is not locally known and is simply absent — no error.
	repo := &familyRepoFake{
		parents: map[string]string{"X": "did:remote:frame", "Y": "X"},
		known:   map[string]bool{"X": true, "Y": true},
	}

	family, err := resolveFamily(context.Background(), nil, repo, "Y")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !reflect.DeepEqual(family, []string{"X", "Y"}) {
		t.Fatalf("got %v", family)
	}
}

func TestResolveFamilyCycleSafe(t *testing.T) {
	repo := &familyRepoFake{
		parents: map[string]string{"A": "B", "B": "A"},
		known:   map[string]bool{"A": true, "B": true},
	}

	family, err := resolveFamily(context.Background(), nil, repo, "A")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(family) != 2 {
		t.Fatalf("expected both cycle members exactly once, got %v", family)
	}
}

func TestResolveFamilyUnknownContract(t *testing.T) {
	repo := &familyRepoFake{parents: map[string]string{}, known: map[string]bool{}}

	_, err := resolveFamily(context.Background(), nil, repo, "nope")
	refused, ok := AsRefused(err)
	if !ok || !strings.Contains(refused.Error(), "not found") {
		t.Fatalf("expected a not-found refusal, got %v", err)
	}
}

func TestReadableRelatedFiltersUnreadableMember(t *testing.T) {
	// Sibling "C" exists locally but is not readable by the requester: it is
	// silently omitted — no error — exactly like a non-local member.
	repo := &familyRepoFake{
		parents:   map[string]string{"B": "A", "C": "A"},
		known:     map[string]bool{"A": true, "B": true, "C": true},
		createdBy: map[string]string{"A": "org-1", "B": "org-1", "C": "org-2"},
	}
	family, err := resolveFamily(context.Background(), nil, repo, "B")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	packaged := map[string]bool{"A": true, "B": true} // requested contract + parent chain

	mayRead := func(data *cwedb.Contract) bool { return data.CreatedBy == "org-1" }
	related, err := readableRelated(context.Background(), nil, repo, family, packaged, mayRead)
	if err != nil {
		t.Fatalf("readableRelated: %v", err)
	}
	if len(related) != 0 {
		t.Fatalf("unreadable sibling must be omitted without error, got %v", related)
	}

	// The same sibling IS included once the requester may read it.
	related, err = readableRelated(context.Background(), nil, repo, family, packaged, func(*cwedb.Contract) bool { return true })
	if err != nil {
		t.Fatalf("readableRelated: %v", err)
	}
	if !reflect.DeepEqual(related, []string{"C"}) {
		t.Fatalf("readable sibling must be included, got %v", related)
	}
}

func TestZipWithManifestIndexesRelatedEntries(t *testing.T) {
	files := bundleFiles{
		"contract.jsonld":                       []byte(`{"@id":"did:example:b"}`),
		"contract.pdf":                          []byte("%PDF-1.7 fake"),
		"parents/did:example:a/contract.pdf":    []byte("%PDF-1.7 parent"),
		"related/did:example:c/contract.jsonld": []byte(`{"@id":"did:example:c"}`),
		"related/did:example:c/contract.pdf":    []byte("%PDF-1.7 sibling"),
	}
	components := []componentInfo{
		{DID: "did:example:b", State: "SIGNED", Role: "root", ParentDID: "did:example:a"},
		{DID: "did:example:a", State: "SIGNED", Role: "parent"},
		{DID: "did:example:c", State: "DRAFT", Role: "related", ParentDID: "did:example:a"},
	}

	zipBytes, err := zipWithManifest(files, "contract", "did:example:b", components)
	if err != nil {
		t.Fatalf("zipWithManifest: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	contents := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		contents[f.Name] = data
	}

	var manifest bundleManifest
	if err := json.Unmarshal(contents["bundle-manifest.json"], &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	indexed := map[string]string{}
	for _, e := range manifest.Entries {
		indexed[e.Path] = e.SHA256
	}
	for _, want := range []string{"related/did:example:c/contract.jsonld", "related/did:example:c/contract.pdf"} {
		sha, ok := indexed[want]
		if !ok {
			t.Fatalf("manifest does not index related entry %s: %v", want, indexed)
		}
		sum := sha256.Sum256(contents[want])
		if hex.EncodeToString(sum[:]) != sha {
			t.Fatalf("sha256 mismatch for related entry %s", want)
		}
	}
}
