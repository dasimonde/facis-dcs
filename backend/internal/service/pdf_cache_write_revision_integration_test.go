package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
)

// A background PDF cache write must not move the revision that the optimistic
// lock compares; an edit must. The waits put the writes into a later second,
// because the lock compares seconds.
func TestPDFCacheWriteKeepsRevisionPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,template_data) VALUES ('tpl','creator','DRAFT','CONTRACT_TEMPLATE','SINGLE_PARTY','{"x":"hello world"}')`)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data) VALUES ('ctr','','Org A','DRAFT','tpl',1,'{"x":"hello world"}')`)
	require.NoError(t, err)

	for _, table := range []string{"contract_templates", "contracts"} {
		t.Run(table, func(t *testing.T) {
			revision := func() time.Time {
				var at time.Time
				require.NoError(t, database.Get(&at, `SELECT updated_at FROM `+table+` WHERE did IN ('tpl','ctr')`))
				return at
			}
			before := revision()
			time.Sleep(1100 * time.Millisecond)
			_, err := database.Exec(`UPDATE ` + table + ` SET pdf_ipfs_cid='cid', pdf_renderer_version='v', pdf_c2pa_state='SIGNED', pdf_payload_hash='h' WHERE did IN ('tpl','ctr')`)
			require.NoError(t, err)
			require.Equal(t, before, revision(), "the PDF cache write moved updated_at")

			_, err = database.Exec(`UPDATE ` + table + ` SET created_by='editor' WHERE did IN ('tpl','ctr')`)
			require.NoError(t, err)
			require.True(t, revision().After(before), "an edit must move updated_at")
		})
	}
}
