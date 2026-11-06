package bundleexport

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	tplpg "digital-contracting-service/internal/templaterepository/db/pg"
)

// DCS-FR-TR-24, DCS-FR-TR-23, DCS-FR-TR-09: the template bundle holds the
// template, the templates it requires (also through a required template) and
// the provenance credentials of each.
func TestTemplateBundleAssemblesRequiredTemplatesPostgres(t *testing.T) {
	database := pgtest.Open(t)
	insert := func(did, level string, requires ...string) {
		t.Helper()
		_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name,template_data,template_level,required_templates)
			VALUES ($1,'creator','REGISTERED','CONTRACT_TEMPLATE','SINGLE_PARTY','n',$2,$3,COALESCE($4::text[],'{}'))`, did, `{"name":"`+did+`"}`, level, pq.Array(requires))
		require.NoError(t, err)
		_, err = database.Exec(`INSERT INTO template_provenance_credentials (did,version,vc_id,credential) VALUES ($1,1,$2,$3)`, did, "urn:vc:"+did, `{"id":"urn:vc:`+did+`"}`)
		require.NoError(t, err)
	}
	insert("frame", "FRAME_AGREEMENT")
	insert("agreement", "AGREEMENT", "frame")
	insert("addendum", "ADDENDUM", "agreement")

	b := &Bundler{DB: database, TRepo: &tplpg.PostgresContractTemplateRepo{},
		templatePDF: func(_ context.Context, did string) ([]byte, error) { return []byte("%PDF " + did), nil }}
	reader, err := b.ExportTemplate(context.Background(), "addendum")
	require.NoError(t, err)
	raw, err := io.ReadAll(reader)
	require.NoError(t, err)
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	require.NoError(t, err)
	names := map[string]bool{}
	for _, file := range archive.File {
		names[file.Name] = true
	}
	for _, name := range []string{
		"template.jsonld", "template.pdf", "provenance/v1.vc.json",
		"required/agreement/template.jsonld", "required/agreement/template.pdf", "required/agreement/provenance/v1.vc.json",
		"required/frame/template.jsonld", "required/frame/provenance/v1.vc.json",
	} {
		require.True(t, names[name], "missing %s in %v", name, names)
	}

	manifest, err := archive.Open("bundle-manifest.json")
	require.NoError(t, err)
	var index bundleManifest
	require.NoError(t, json.NewDecoder(manifest).Decode(&index))
	parents := map[string]string{}
	for _, component := range index.Components {
		parents[component.DID] = component.ParentDID
	}
	require.Equal(t, map[string]string{"addendum": "", "agreement": "addendum", "frame": "agreement"}, parents)
}
