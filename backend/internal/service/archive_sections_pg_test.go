package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/middleware"
)

const sectionedContract = `{"dcs:parties":[{"dcs:legalName":"Org B"},{"dcs:legalName":"Org C"}],
	"dcs:documentStructure":{"dcs:blocks":{"@list":[
		{"@id":"urn:c#payment","@type":"dcs:Clause","dcs:title":"Payment","dcs:content":"Org B pays 100 EUR"},
		{"@id":"urn:c#delivery","@type":"dcs:Section","dcs:title":"Delivery","dcs:content":"Org C delivers"}]}}}`

func organizationContext(organization, role string) context.Context {
	return middleware.InjectAuthContext(context.Background(), []string{role}, "did:key:holder", organization)
}

// DCS-FR-CSA-26: each party's assigned sections are archived one by one,
// linked to the contract's archive entry, and each party reads only its own
// sections. The Auditor reads all of them. Every read is audited.
func TestArchivedSectionsAreReadableOnlyByTheirParties(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:dcs.example:contract:sections"
	_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, contract_data)
		VALUES ($1, 'did:web:dcs.example', 'did:web:creator', 'SIGNED', 'did:web:tpl', 1, $2)`, did, sectionedContract)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contract_archive_entries (did, contract_version, stored_by, contract_snapshot, content_hash, snapshot_cid, evidence)
		VALUES ($1::text, 1, 'did:web:creator', jsonb_build_object('did', $1::text, 'contract_data', $2::jsonb), 'sha256:`+zeroHash+`', 'bafy-snapshot',
		        '{"source":"SIGNING_WORKFLOW_COMPLETION","snapshot_hash_algorithm":"SHA-256"}')`, did, sectionedContract)
	require.NoError(t, err)
	svc := archiveTestService(t, db)
	artifacts := newMemoryArtifacts()
	svc.Artifacts = artifacts

	store := func(section string, parties ...string) error {
		_, err := svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{Kind: "section", Did: did, SectionID: &section, Parties: parties})
		return err
	}
	require.NoError(t, store("urn:c#payment", "Org B"))
	require.NoError(t, store("urn:c#delivery", "Org C", archiveManager))
	require.ErrorContains(t, store("urn:c#payment", "Org B"), "already archived")
	require.ErrorContains(t, store("urn:c#delivery", "Org X"), "not a party")
	require.ErrorContains(t, store("urn:c#missing", "Org B"), "has no section")
	require.Len(t, archiveEvents(t, db, "STORE_ARCHIVE_SECTION"), 2)

	read := func(ctx context.Context, section string) ([]string, error) {
		payload := &contractstoragearchive.SectionsPayload{Did: did}
		if section != "" {
			payload.SectionID = &section
		}
		items, err := svc.Sections(ctx, payload)
		ids := make([]string, 0, len(items))
		for _, item := range items {
			require.Equal(t, 1, item.ContractVersion)
			if item.Content != nil {
				ids = append(ids, item.SectionID)
			}
		}
		return ids, err
	}
	orgB := organizationContext("Org B", "Contract Observer")
	ids, err := read(orgB, "")
	require.NoError(t, err)
	require.Equal(t, []string{"urn:c#payment"}, ids)
	ids, err = read(organizationContext("Org C", "Contract Observer"), "")
	require.NoError(t, err)
	require.Equal(t, []string{"urn:c#delivery"}, ids)
	ids, err = read(organizationContext("Org Audit", "Auditor"), "")
	require.NoError(t, err)
	require.Equal(t, []string{"urn:c#payment", "urn:c#delivery"}, ids)

	_, err = read(orgB, "urn:c#delivery")
	require.ErrorContains(t, err, "not assigned")
	failed := archiveEvents(t, db, "ARCHIVE_OPERATION_FAILED")
	require.Equal(t, "sections", failed[len(failed)-1]["operation"])
	retrieved := archiveEvents(t, db, "RETRIEVE_ARCHIVED_CONTRACTS")
	require.Equal(t, []any{"urn:c#payment"}, retrieved[0]["section_ids"])
	require.Equal(t, "Org B", retrieved[0]["retrieved_by"])

	artifacts.shredded[did] = true
	items, err := svc.Sections(orgB, &contractstoragearchive.SectionsPayload{Did: did})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Nil(t, items[0].Content, "erased section content must not be returned")

	_, err = db.Exec(`UPDATE contract_archive_sections SET parties = '{"Org X"}'`)
	require.ErrorContains(t, err, "append-only")

	// The deletion of the archived contract removes the section content
	// from IPFS with the snapshots (DCS-NFR-SEC-13).
	var sectionCIDs []string
	require.NoError(t, db.Select(&sectionCIDs, `SELECT content_cid FROM contract_archive_sections WHERE did = $1`, did))
	snapshots := &recordingSnapshotStore{}
	svc.SnapshotStore, svc.Eraser = snapshots, noopEraser{}
	_, err = db.Exec(`UPDATE contracts SET state = 'TERMINATED' WHERE did = $1`, did)
	require.NoError(t, err)
	_, err = svc.Delete(archiveManagerContext(), &contractstoragearchive.DeletePayload{Did: did, Justification: "retention ended"})
	require.NoError(t, err)
	require.Subset(t, snapshots.deleted, sectionCIDs)
}

// DCS-FR-CSA-26: a section is archived into the archived version it is
// taken from, not always into the latest one.
func TestSectionIsArchivedIntoTheGivenVersion(t *testing.T) {
	db := pgtest.Open(t)
	did := "did:web:dcs.example:contract:versions"
	_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, contract_data, contract_version)
		VALUES ($1, 'did:web:dcs.example', 'did:web:creator', 'SIGNED', 'did:web:tpl', 1, $2, 2)`, did, sectionedContract)
	require.NoError(t, err)
	for version, data := range map[int]string{1: sectionedContract, 2: `{"dcs:parties":[{"dcs:legalName":"Org B"}]}`} {
		_, err = db.Exec(`INSERT INTO contract_archive_entries (did, contract_version, stored_by, contract_snapshot, content_hash, snapshot_cid, evidence)
			VALUES ($1::text, $2, 'did:web:creator', jsonb_build_object('contract_data', $3::jsonb), 'sha256:`+zeroHash+`', 'bafy', '{"source":"test","snapshot_hash_algorithm":"SHA-256"}')`, did, version, data)
		require.NoError(t, err)
	}
	svc := archiveTestService(t, db)
	svc.Artifacts = newMemoryArtifacts()
	section, first := "urn:c#payment", 1

	_, err = svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{Kind: "section", Did: did, SectionID: &section, Parties: []string{"Org B"}})
	require.ErrorContains(t, err, "version 2", "without a version the latest archived version is used")
	res, err := svc.Store(archiveManagerContext(), &contractstoragearchive.StorePayload{Kind: "section", Did: did, SectionID: &section, Parties: []string{"Org B"}, ContractVersion: &first})
	require.NoError(t, err)
	require.Equal(t, 1, *res.ContractVersion)
}

type noopEraser struct{}

func (noopEraser) EraseContract(context.Context, string, string, string) error { return nil }
