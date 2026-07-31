package command

import (
	"context"
	"testing"

	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/stretchr/testify/require"
)

type fakeTargetSeedRepo struct {
	existing []db.ContractTarget
	created  []db.ContractTarget
}

func (f *fakeTargetSeedRepo) list(_ context.Context) ([]db.ContractTarget, error) {
	return f.existing, nil
}

func (f *fakeTargetSeedRepo) create(_ context.Context, target db.ContractTarget) error {
	f.created = append(f.created, target)
	f.existing = append(f.existing, target)
	return nil
}

// A fresh install has an empty registry, so a contract could not be pointed
// anywhere until somebody opened the admin UI. Seeding from deployment
// configuration is what makes an instance usable straight after install.
func TestSeedTargetsRegistersConfiguredEntries(t *testing.T) {
	repo := &fakeTargetSeedRepo{}
	seeded, err := seedTargets(context.Background(), repo.list, repo.create, []SeedTarget{
		{Name: "ORCE", URL: "http://dcs-orce:1880/contract-target/deploy"},
	})

	require.NoError(t, err)
	require.Equal(t, 1, seeded)
	require.Len(t, repo.created, 1)
	require.Equal(t, "ORCE", repo.created[0].Name)
	require.True(t, repo.created[0].Enabled, "a seeded target accepts deployments unless it says otherwise")
}

// The seed must not fight the administrator. An operator who repoints a target
// in the UI would otherwise have it silently reverted by the next restart —
// deployments would go somewhere they were explicitly moved away from.
func TestSeedTargetsLeavesAnExistingEntryAlone(t *testing.T) {
	repo := &fakeTargetSeedRepo{existing: []db.ContractTarget{
		{ID: "1", Name: "ORCE", URL: "http://edited-by-the-admin:9999/deploy", Enabled: false},
	}}
	seeded, err := seedTargets(context.Background(), repo.list, repo.create, []SeedTarget{
		{Name: "ORCE", URL: "http://dcs-orce:1880/contract-target/deploy"},
	})

	require.NoError(t, err)
	require.Equal(t, 0, seeded)
	require.Empty(t, repo.created)
	require.Equal(t, "http://edited-by-the-admin:9999/deploy", repo.existing[0].URL)
	require.False(t, repo.existing[0].Enabled)
}

// Seeding runs on every start, so it must be idempotent by name rather than
// accumulating duplicates a manager would then have to choose between.
func TestSeedTargetsIsIdempotent(t *testing.T) {
	repo := &fakeTargetSeedRepo{}
	entries := []SeedTarget{{Name: "ORCE", URL: "http://dcs-orce:1880/contract-target/deploy"}}

	_, err := seedTargets(context.Background(), repo.list, repo.create, entries)
	require.NoError(t, err)
	seeded, err := seedTargets(context.Background(), repo.list, repo.create, entries)
	require.NoError(t, err)

	require.Equal(t, 0, seeded)
	require.Len(t, repo.existing, 1)
}

func TestSeedTargetsRejectsAnEntryWithoutNameOrURL(t *testing.T) {
	repo := &fakeTargetSeedRepo{}
	_, err := seedTargets(context.Background(), repo.list, repo.create, []SeedTarget{{Name: "ORCE"}})
	require.Error(t, err, "a target with no URL would fail every dispatch at run time instead of at install")
}

func TestParseSeedTargetsReadsTheConfiguredList(t *testing.T) {
	entries, err := ParseSeedTargets([]byte(`[
        {"name":"ORCE","url":"http://dcs-orce:1880/contract-target/deploy","description":"shipped flow"},
        {"name":"ERP","url":"https://erp.example/deploy","enabled":false}
    ]`))

	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "ERP", entries[1].Name)
	require.NotNil(t, entries[1].Enabled)
	require.False(t, *entries[1].Enabled)
}
