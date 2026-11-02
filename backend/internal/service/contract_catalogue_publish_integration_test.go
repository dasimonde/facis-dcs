package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/middleware"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
)

// DCS-IR-CWE-10: an approved contract is registered in the Federated Catalogue
// with its metadata only, and the registration is audited. A draft is refused
// before the catalogue is called.
func TestContractCataloguePublishPostgres(t *testing.T) {
	database := pgtest.Open(t)
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data) VALUES
		('approved',$1,'Org A','APPROVED','tpl-1',1,'Hosting','{"dcs:parties":[{"dcs:legalName":"Org B"}],"clause":"secret terms"}'),
		('draft',$1,'Org A','DRAFT','tpl-1',1,'Draft','{}')`, peer)
	require.NoError(t, err)

	var published []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/realms/fc/protocol/openid-connect/token":
			_, _ = w.Write([]byte(`{"access_token":"token"}`))
		case r.Method == http.MethodPost && r.URL.Path == fcclient.AssetsEndpointPath:
			body, _ := io.ReadAll(r.Body)
			var payload map[string]any
			_ = json.Unmarshal(body, &payload)
			published = append(published, payload)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"assetHash":"asset-1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	fc, err := fcclient.NewFederatedCatalogueClient(fcclient.Config{APIURL: server.URL, KeycloakRealmURL: server.URL + "/realms/fc", ClientID: "dcs", ClientSecret: "secret"})
	require.NoError(t, err)

	svc := &contractWorkflowEnginesrvc{DB: database, CRepo: &cwepg.PostgresContractRepo{}, DIDDocument: *document, FCClient: fc}
	ctx := middleware.InjectAuthContext(context.Background(), []string{"Contract Approver"}, "holder", "Org A")

	res, err := svc.PublishCatalogue(ctx, &cwe.ContractCataloguePublishRequest{Did: "approved"})
	require.NoError(t, err)
	require.Equal(t, "asset-1", res.AssetHash)
	require.Len(t, published, 1)
	subject := published[0]["credentialSubject"].(map[string]any)
	require.Equal(t, "approved", subject["dcs:contractUuid"])
	require.Equal(t, "approved", subject["dcs:state"])
	require.Equal(t, "Hosting", subject["schema:name"])
	require.Equal(t, "tpl-1", subject["dcs:derivedFromTemplate"])
	require.NotContains(t, subject, "dcs:templateUuid", "a contract entry must not show up as a template")
	raw, err := json.Marshal(published[0])
	require.NoError(t, err)
	require.NotContains(t, string(raw), "secret terms")
	require.NotContains(t, string(raw), "Org B")

	var audited []string
	require.NoError(t, database.Select(&audited, `SELECT event_data::text FROM outbox_events WHERE event_type = 'PUBLISH_CONTRACT_TO_CATALOGUE'`))
	require.Len(t, audited, 1)
	var evt map[string]any
	require.NoError(t, json.Unmarshal([]byte(audited[0]), &evt))
	require.Equal(t, "approved", evt["did"])
	require.Equal(t, "Org A", evt["published_by"])
	require.Equal(t, "asset-1", evt["asset_hash"])

	_, err = svc.PublishCatalogue(ctx, &cwe.ContractCataloguePublishRequest{Did: "draft"})
	var refused *goa.ServiceError
	require.True(t, errors.As(err, &refused) && refused.Name == "bad_request", "draft: %v", err)
	require.Len(t, published, 1)
}
