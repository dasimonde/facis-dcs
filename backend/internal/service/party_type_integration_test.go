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

	server "digital-contracting-service/gen/http/template_repository/server"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/semantichub/hubtest"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/db"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
)

func TestPartyTypePostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	repo := &pg.PostgresContractTemplateRepo{}
	for _, fixture := range []struct{ did, kind, party string }{
		{"single", "CONTRACT_TEMPLATE", "SINGLE_PARTY"},
		{"multi", "CONTRACT_TEMPLATE", "MULTI_PARTY"},
		{"component", "COMPONENT", "MULTI_PARTY"},
	} {
		_, err := repo.Create(ctx, tx, db.ContractTemplate{DID: fixture.did, CreatedBy: "tester", State: "DRAFT", TemplateType: fixture.kind, PartyType: fixture.party})
		require.NoError(t, err)
		row, err := repo.ReadDataByID(ctx, tx, fixture.did)
		require.NoError(t, err)
		require.Equal(t, fixture.party, row.PartyType)
	}
	rows, err := repo.ReadAllMetaDataByFilter(ctx, tx, db.SearchValues{TemplateType: "CONTRACT_TEMPLATE", PartyType: "MULTI_PARTY"}, datatype.Pagination{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "multi", rows[0].DID)
	require.NoError(t, repo.CreateHistoryEntryForDID(ctx, tx, "single"))
	require.NoError(t, repo.Update(ctx, tx, db.ContractTemplateUpdateData{DID: "single", PartyType: "MULTI_PARTY"}))
	name := "renamed"
	require.NoError(t, repo.Update(ctx, tx, db.ContractTemplateUpdateData{DID: "single", Name: &name}))
	row, err := repo.ReadDataByID(ctx, tx, "single")
	require.NoError(t, err)
	require.Equal(t, "MULTI_PARTY", row.PartyType)
	history, err := repo.ReadHistoryByDID(ctx, tx, "single")
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, "SINGLE_PARTY", history[0].PartyType)
	_, err = repo.CopyFromDID(ctx, tx, "single", "copy")
	require.NoError(t, err)
	row, err = repo.ReadDataByID(ctx, tx, "copy")
	require.NoError(t, err)
	require.Equal(t, "MULTI_PARTY", row.PartyType)
	require.NoError(t, repo.UpdateState(ctx, tx, "single", "DRAFT", "REGISTERED", "registrar"))
	version, err := repo.CopyFromDID(ctx, tx, "single", "version-two")
	require.NoError(t, err)
	require.Equal(t, 2, version)
	row, err = repo.ReadDataByID(ctx, tx, "version-two")
	require.NoError(t, err)
	require.Equal(t, "MULTI_PARTY", row.PartyType)
	require.NoError(t, tx.Commit())
	updater := &command.Updater{DB: database, CTRepo: repo}
	require.ErrorContains(t, updater.Handle(ctx, command.UpdateCmd{DID: "copy", PartyType: "SINGLE_PARTY", UpdatedAt: time.Now().Add(time.Hour)}), "permission")
	require.ErrorContains(t, updater.Handle(ctx, command.UpdateCmd{DID: "single", PartyType: "SINGLE_PARTY", UpdatedAt: time.Now().Add(time.Hour)}), "state")
	require.ErrorContains(t, updater.Handle(ctx, command.UpdateCmd{DID: "copy", PartyType: "SINGLE_PARTY", UpdatedAt: time.Time{}}), "reload")
	manager := &command.UpdateManager{DB: database, CTRepo: repo, RTRepo: &pg.PostgresReviewTaskRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}}
	require.ErrorContains(t, manager.Handle(ctx, command.UpdateManageCmd{DID: "single", PartyType: "SINGLE_PARTY", UpdatedAt: time.Now().Add(time.Hour)}), "state")
	require.ErrorContains(t, manager.Handle(ctx, command.UpdateManageCmd{DID: "copy", PartyType: "SINGLE_PARTY", UpdatedAt: time.Time{}}), "reload")

}

func TestPartyTypeCreateValidation(t *testing.T) {
	kind := "CONTRACT_TEMPLATE"
	require.Error(t, server.ValidateCreateRequestBody(&server.CreateRequestBody{TemplateType: &kind}))
	for _, value := range []string{"", "multi_party", "CONTRACT_TEMPLATE", "MULTI_PARTY", "SINGLE_PARTY"} {
		err := server.ValidateCreateRequestBody(&server.CreateRequestBody{TemplateType: &kind, PartyType: &value})
		if value == "MULTI_PARTY" || value == "SINGLE_PARTY" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}

func TestPartyTypeUpdateManageHistory(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	repo := &pg.PostgresContractTemplateRepo{}
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = repo.Create(ctx, tx, db.ContractTemplate{DID: "managed", CreatedBy: "tester", State: "DRAFT", TemplateType: "CONTRACT_TEMPLATE", PartyType: "SINGLE_PARTY"})
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	manager := &command.UpdateManager{DB: database, CTRepo: repo, RTRepo: &pg.PostgresReviewTaskRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}}
	var expectedHistory []string
	for index, change := range []struct{ value, expected, prior string }{
		{"MULTI_PARTY", "MULTI_PARTY", "SINGLE_PARTY"},
		{"SINGLE_PARTY", "SINGLE_PARTY", "MULTI_PARTY"},
		{"", "SINGLE_PARTY", "SINGLE_PARTY"},
	} {
		name := "managed update"
		require.NoError(t, manager.Handle(ctx, command.UpdateManageCmd{DID: "managed", UpdatedAt: time.Now().Add(time.Hour), PartyType: change.value, Name: &name, UpdatedBy: "tester"}))
		var party string
		require.NoError(t, database.Get(&party, `SELECT party_type FROM contract_templates WHERE did='managed'`))
		require.Equal(t, change.expected, party)
		var history []string
		require.NoError(t, database.Select(&history, `SELECT party_type FROM contract_templates_history WHERE did='managed' ORDER BY id`))
		require.Len(t, history, index+1)
		expectedHistory = append(expectedHistory, change.prior)
		require.ElementsMatch(t, expectedHistory, history)
		var events int
		require.NoError(t, database.Get(&events, `SELECT COUNT(*) FROM outbox_events WHERE did='managed'`))
		require.Equal(t, index+1, events)
	}
}

func TestPartyTypeCatalogueImport(t *testing.T) {
	database := hubtest.Open(t)
	for _, value := range []string{"SINGLE_PARTY", "MULTI_PARTY", "", "invalid"} {
		t.Run(value, func(t *testing.T) {
			source := "source-" + value
			fc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if strings.HasSuffix(r.URL.Path, "/token") {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "expires_in": 300, "token_type": "Bearer"}))
					return
				}
				require.Equal(t, "/query/search", r.URL.Path)
				require.Equal(t, "POST", r.Method)
				row := map[string]interface{}{"did": source, "version": 1, "template_type": "CONTRACT_TEMPLATE", "name": "imported", "template_data_string": compliantTemplateData}
				if value != "" {
					row["party_type"] = value
				}
				require.NoError(t, json.NewEncoder(w).Encode(fcclient.QueryResults{TotalCount: 1, Items: []map[string]interface{}{row}}))
			}))
			defer fc.Close()
			client, err := fcclient.NewFederatedCatalogueClient(fcclient.Config{APIURL: fc.URL, KeycloakRealmURL: fc.URL + "/realms/test", ClientID: "test", ClientSecret: "test"})
			require.NoError(t, err)
			registrar := &command.Registrar{DB: database, CTRepo: &pg.PostgresContractTemplateRepo{}, FCClient: client}
			var before int
			require.NoError(t, database.Get(&before, `SELECT COUNT(*) FROM contract_templates`))
			did, err := registrar.Handle(context.Background(), command.RegisterCmd{DID: source, Version: 1, RegisteredBy: "tester"})
			if value == "" || value == "invalid" {
				require.ErrorContains(t, err, "party type")
				require.Nil(t, did)
				var after, events int
				require.NoError(t, database.Get(&after, `SELECT COUNT(*) FROM contract_templates`))
				require.Equal(t, before, after)
				require.NoError(t, database.Get(&events, `SELECT COUNT(*) FROM outbox_events WHERE event_data->>'source_did'=$1`, source))
				require.Zero(t, events)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, did)
			var party string
			require.NoError(t, database.Get(&party, `SELECT party_type FROM contract_templates WHERE did=$1`, *did))
			require.Equal(t, value, party)
			var events int
			require.NoError(t, database.Get(&events, `SELECT COUNT(*) FROM outbox_events WHERE did=$1`, *did))
			require.Equal(t, 1, events)
		})
	}
}
