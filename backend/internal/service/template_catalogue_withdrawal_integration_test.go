package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/command"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
)

// DCS-NFR-BR-09: publishing records the catalogue asset; deprecating the
// template revokes it, and a failed revocation keeps the template published.
func TestTemplateDeprecationWithdrawsCatalogueAssetPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	revokeStatus := http.StatusOK
	var revoked []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/realms/fc/protocol/openid-connect/token":
			_, _ = w.Write([]byte(`{"access_token":"token"}`))
		case r.Method == http.MethodPost && r.URL.Path == fcclient.AssetsEndpointPath:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"assetHash":"hash-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == fcclient.AssetsEndpointPath+"/hash-1/revoke":
			revoked = append(revoked, "hash-1")
			w.WriteHeader(revokeStatus)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	fc, err := fcclient.NewFederatedCatalogueClient(fcclient.Config{APIURL: server.URL, KeycloakRealmURL: server.URL + "/realms/fc", ClientID: "dcs", ClientSecret: "secret"})
	require.NoError(t, err)

	ctRepo := &pg.PostgresContractTemplateRepo{}
	_, err = database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name,description,template_data,compliance_status) VALUES ('tpl','creator','REGISTERED','CONTRACT_TEMPLATE','SINGLE_PARTY','n','d',$1,'COMPLIANT')`, compliantTemplateData)
	require.NoError(t, err)
	future := time.Now().Add(time.Hour)
	require.NoError(t, (&command.Publisher{DB: database, CTRepo: ctRepo, FCClient: fc}).Handle(ctx, command.PublishCmd{DID: "tpl", UpdatedAt: future, PublishedBy: "manager", InstanceDID: "did:web:dcs.example"}))
	var hash string
	require.NoError(t, database.Get(&hash, `SELECT catalogue_asset_hash FROM contract_templates WHERE did='tpl'`))
	require.Equal(t, "hash-1", hash)

	archiver := &command.Archiver{DB: database, CTRepo: ctRepo, RTRepo: &pg.PostgresReviewTaskRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}, FCClient: fc}
	state := func() string {
		var s string
		require.NoError(t, database.Get(&s, `SELECT state FROM contract_templates WHERE did='tpl'`))
		return s
	}

	revokeStatus = http.StatusInternalServerError
	require.Error(t, archiver.Handle(ctx, command.ArchiveCmd{DID: "tpl", UpdatedAt: future, ArchivedBy: "manager", Justification: "withdrawn"}))
	require.Equal(t, "PUBLISHED", state())

	revokeStatus = http.StatusOK
	require.NoError(t, archiver.Handle(ctx, command.ArchiveCmd{DID: "tpl", UpdatedAt: future, ArchivedBy: "manager", Justification: "withdrawn"}))
	require.Equal(t, "DEPRECATED", state())
	require.Equal(t, []string{"hash-1", "hash-1"}, revoked)
}
