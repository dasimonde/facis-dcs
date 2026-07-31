package semantichub

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeHubRows answers the two single-row reads resolveVersion makes, standing
// in for one instance's semantic_schemas table.
type fakeHubRows struct {
	highest int
	taken   map[int]bool
}

func (f fakeHubRows) GetContext(_ context.Context, dest any, query string, args ...any) error {
	switch {
	case strings.Contains(query, "EXISTS"):
		version, ok := args[2].(int)
		if !ok {
			return fmt.Errorf("version argument is %T, not int", args[2])
		}
		target, ok := dest.(*bool)
		if !ok {
			return fmt.Errorf("destination is %T, not *bool", dest)
		}
		*target = f.taken[version]
		return nil
	case strings.Contains(query, "MAX(version)"):
		target, ok := dest.(*int)
		if !ok {
			return fmt.Errorf("destination is %T, not *int", dest)
		}
		*target = f.highest
		return nil
	}
	return fmt.Errorf("unexpected query: %s", query)
}

// A template pins its shape libraries by version (ADR-8), and the pin travels
// with the template across the Federated Catalogue. The consuming instance
// has its own version history for that library — it installed the library
// later, or fewer times, than the publisher revised it — so unless the
// consumer can choose the number, the publisher's pin names a version that
// instance can never hold, and every contract derived from the template fails
// validation.
func TestRegisterLandsAPeerLibraryAtThePeersVersionNumber(t *testing.T) {
	// The publisher revised the library twice and its template pins version 3;
	// this instance installed it once, so its own next version would be 2.
	hub := fakeHubRows{highest: 1, taken: map[int]bool{1: true}}

	assigned, err := resolveVersion(context.Background(), hub, "e2e-sla-shapes", "shapes", 3)
	require.NoError(t, err)
	require.Equal(t, 3, assigned, "the peer's library must land at the version its pin names, not this hub's next number")
}

func TestRegisterRefusesAVersionThisHubAlreadyHolds(t *testing.T) {
	hub := fakeHubRows{highest: 1, taken: map[int]bool{1: true}}

	_, err := resolveVersion(context.Background(), hub, "e2e-sla-shapes", "shapes", 1)
	require.ErrorIs(t, err, ErrVersionTaken)
	require.ErrorContains(t, err, "e2e-sla-shapes/shapes version 1")
}

func TestRegisterWithoutARequestedVersionTakesTheNextOne(t *testing.T) {
	hub := fakeHubRows{highest: 1, taken: map[int]bool{1: true}}

	assigned, err := resolveVersion(context.Background(), hub, "e2e-sla-shapes", "shapes", 0)
	require.NoError(t, err)
	require.Equal(t, 2, assigned)
}

func TestRegisterOfAnEntryThisHubDoesNotHoldStartsAtOne(t *testing.T) {
	assigned, err := resolveVersion(context.Background(), fakeHubRows{}, "e2e-sla-shapes", "shapes", 0)
	require.NoError(t, err)
	require.Equal(t, 1, assigned)
}

func TestRegisterRejectsANegativeVersion(t *testing.T) {
	_, err := resolveVersion(context.Background(), fakeHubRows{}, "e2e-sla-shapes", "shapes", -1)
	require.ErrorContains(t, err, "must not be negative")
}
