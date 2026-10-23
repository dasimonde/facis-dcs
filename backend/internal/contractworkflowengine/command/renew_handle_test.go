package command

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// renewableDocument is signedTermDocument with the block ID and layout that
// persistence validation requires.
func renewableDocument(t *testing.T) *datatype.JSON {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(*signedTermDocument(t)), &doc))
	const clause = "did:web:facis.example:contract:1#clause-1"
	structure := doc["dcs:documentStructure"].(map[string]any)
	structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)["@id"] = clause
	structure["dcs:layout"] = []any{
		map[string]any{
			"@id":          "did:web:facis.example:contract:1#root",
			"dcs:isRoot":   true,
			"dcs:children": map[string]any{"@list": []any{map[string]any{"@id": clause}}},
		},
		map[string]any{"@id": clause, "dcs:children": map[string]any{"@list": []any{}}},
	}
	encoded, err := datatype.NewJSON(doc)
	require.NoError(t, err)
	return &encoded
}

type renewRepoFake struct {
	*taskContractRepoFake
	created *db.Contract
}

func (r *renewRepoFake) Create(_ context.Context, _ *sqlx.Tx, data db.Contract) error {
	r.created = &data
	return nil
}

// Renewing an expired contract writes the moved term to the row and into the
// renewal's dcs:metadata, so the DRAFT does not read EXPIRED at once.
func TestRenewerMovesAnExpiredTermForward(t *testing.T) {
	now := time.Now().UTC()
	origStart := now.AddDate(-1, 0, -10)
	origExp := now.AddDate(0, 0, -10)
	repo := &renewRepoFake{taskContractRepoFake: &taskContractRepoFake{stored: &db.Contract{
		DID:             "did:web:facis.example:contract:1",
		State:           "EXPIRED",
		ContractVersion: 2,
		UpdatedAt:       now.Add(-time.Hour),
		ContractData:    renewableDocument(t),
		StartDate:       &origStart,
		ExpDate:         &origExp,
	}}}
	renewer := Renewer{DB: taskTestDB(t), CRepo: repo, DIDDocument: *taskTestDIDDocument(t)}

	_, err := renewer.Handle(context.Background(), RenewCmd{
		DID:         "did:web:facis.example:contract:2",
		OriginalDID: "did:web:facis.example:contract:1",
		RenewedBy:   "did:web:facis.example:manager",
		UpdatedAt:   now,
	})
	require.NoError(t, err)

	require.Len(t, repo.updates, 1)
	require.Equal(t, origExp, *repo.updates[0].StartDate)
	require.True(t, repo.updates[0].ExpDate.After(now))

	require.NotNil(t, repo.created)
	meta := renewalMetadata(t, repo.created.ContractData)
	require.Equal(t, repo.updates[0].ExpDate.Format(time.RFC3339), meta[metaExpirationDate].(map[string]any)["@value"])
}

func TestRenewerRejectsAPastExpiry(t *testing.T) {
	now := time.Now().UTC()
	repo := &renewRepoFake{taskContractRepoFake: &taskContractRepoFake{stored: &db.Contract{
		DID: "did:web:facis.example:contract:1", State: "ACTIVE", UpdatedAt: now.Add(-time.Hour), ContractData: renewableDocument(t),
	}}}
	renewer := Renewer{DB: taskTestDB(t), CRepo: repo, DIDDocument: *taskTestDIDDocument(t)}

	past := now.AddDate(0, 0, -1)
	_, err := renewer.Handle(context.Background(), RenewCmd{
		DID: "did:web:facis.example:contract:2", OriginalDID: "did:web:facis.example:contract:1", UpdatedAt: now, NewExpDate: &past,
	})
	require.ErrorIs(t, err, ErrInvalidRenewalTerm)
	require.Nil(t, repo.created)
}
