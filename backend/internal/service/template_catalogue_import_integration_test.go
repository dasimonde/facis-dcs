package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/fcasset"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/command"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
)

// DCS-FR-TR-01, DCS-FR-TR-10, DCS-FR-TR-02: a template imported from the
// Federated Catalogue is stored in the canonical envelope under its own
// resource IRI and keeps the jurisdiction, industry and template level the
// publishing instance put into the catalogue.
func TestTemplateCatalogueImportNormalizesAndKeepsMetadataPostgres(t *testing.T) {
	database := pgtest.Open(t)
	var statement string
	fc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/token") {
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "expires_in": 300, "token_type": "Bearer"})
			return
		}
		var query struct {
			Statement string `json:"statement"`
		}
		_ = json.NewDecoder(r.Body).Decode(&query)
		statement = query.Statement
		row := map[string]any{
			"did": "remote", "version": 1, "template_type": "CONTRACT_TEMPLATE", "party_type": "SINGLE_PARTY", "name": "imported",
			"jurisdiction": "DEU", "industry": "Energy", "template_level": "FRAME_AGREEMENT",
			"template_data_string": strings.Replace(compliantTemplateData, `"@type": "dcs:ContractTemplate",`, `"@id": "https://remote.example/template/remote", "@type": "dcs:ContractTemplate",`, 1),
		}
		_ = json.NewEncoder(w).Encode(fcclient.QueryResults{TotalCount: 1, Items: []map[string]any{row}})
	}))
	defer fc.Close()
	client, err := fcclient.NewFederatedCatalogueClient(fcclient.Config{APIURL: fc.URL, KeycloakRealmURL: fc.URL + "/realms/test", ClientID: "test", ClientSecret: "test"})
	require.NoError(t, err)

	did, err := (&command.Registrar{DB: database, CTRepo: &pg.PostgresContractTemplateRepo{}, FCClient: client}).
		Handle(context.Background(), command.RegisterCmd{DID: "remote", Version: 1, RegisteredBy: "manager"})
	require.NoError(t, err)
	require.Contains(t, statement, "OPTIONAL")

	var stored struct {
		ID           string `db:"id"`
		Jurisdiction string `db:"jurisdiction"`
		Industry     string `db:"industry"`
		Level        string `db:"template_level"`
	}
	require.NoError(t, database.Get(&stored, `SELECT template_data->>'@id' AS id, jurisdiction, industry, template_level FROM contract_templates WHERE did = $1`, *did))
	require.Equal(t, base.ResourceIRI("template", *did), stored.ID)
	require.Equal(t, "DEU", stored.Jurisdiction)
	require.Equal(t, "Energy", stored.Industry)
	require.Equal(t, "FRAME_AGREEMENT", stored.Level)
}

// The catalogue asset carries the searchable metadata the importing instance takes over.
func TestCatalogueAssetCarriesSearchableMetadata(t *testing.T) {
	payload, err := fcasset.BuildPayload(fcasset.BuildInput{
		Issuer: "did:web:dcs.example", SubjectIRI: "https://dcs.example/template/1", ValidFrom: time.Now(),
		Subject: fcasset.CatalogueSubject{ID: "1", PartyType: "SINGLE_PARTY", Jurisdiction: "DEU", Industry: "Energy", TemplateLevel: "ADDENDUM"},
	})
	require.NoError(t, err)
	subject := payload["credentialSubject"].(map[string]any)
	require.Equal(t, "DEU", subject["dcs:jurisdiction"])
	require.Equal(t, "Energy", subject["dcs:industry"])
	require.Equal(t, "ADDENDUM", subject["dcs:templateLevel"])
}
