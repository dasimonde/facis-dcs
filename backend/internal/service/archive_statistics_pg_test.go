package service

import (
	"testing"

	"github.com/stretchr/testify/require"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/base"
	basepg "digital-contracting-service/internal/base/db/pq"
	"digital-contracting-service/internal/base/pgtest"
)

// DCS-FR-CSA-21, DCS-FR-CSA-19, DCS-FR-CSA-04: the dashboard counts an entry
// as compliant only when the compliance tool passes it, and it states its
// expiring window.
func TestArchiveStatisticsCountTheComplianceVerdict(t *testing.T) {
	db := pgtest.Open(t)
	const did = "did:web:dcs.example:contract:evidence-complete"
	// The evidence is complete, but the contract has no validity period and
	// no parties, so the metadata rule flags the entry.
	_, err := db.Exec(`INSERT INTO contracts (did, origin, created_by, state, template_did, template_version, name)
		VALUES ($1, 'did:web:dcs.example', 'did:web:creator', 'SIGNED', 'did:web:tpl', 1, 'n')`, did)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO contract_archive_entries (did, contract_version, stored_by, contract_snapshot, content_hash, snapshot_cid, signature_metadata, tsa_receipt, evidence)
		VALUES ($1, 1, 'did:web:creator', '{"did":"x"}', 'sha256:`+zeroHash+`', 'bafy', '{"status":"SIGNED"}', '{"token":"t"}', '{"source":"test","snapshot_hash_algorithm":"SHA-256"}')`, did)
	require.NoError(t, err)
	svc := archiveTestService(t, db)
	svc.ATrailReader = base.AuditTrailReader{ARepo: &basepg.PostgresAuditTrailRepository{}}

	items, err := svc.Compliance(archiveManagerContext(), &contractstoragearchive.CompliancePayload{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.True(t, items[0].Flagged)

	statistics, err := svc.Statistics(archiveManagerContext(), &contractstoragearchive.StatisticsPayload{})
	require.NoError(t, err)
	require.Equal(t, 0, statistics.CompliantTotal)
	require.Equal(t, 1, statistics.FlaggedTotal)
	require.Equal(t, 30, statistics.ExpiringWindowDays)
}
