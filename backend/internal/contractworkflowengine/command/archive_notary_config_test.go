package command

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The combination a deployment reaches by configuring nothing: the chart
// derives the notary URL from the bundled ORCE release, while the bearer token
// defaults to empty. Left to run, that instance refuses every signature at the
// moment its archive entry is notarized — after the signer has already signed.
func TestArchiveNotaryConfigRefusesAnAddressedNotaryWithoutAToken(t *testing.T) {
	err := ValidateArchiveNotaryConfig("http://dcs-orce:1880/archive/notary", "")

	require.Error(t, err)
	require.Contains(t, err.Error(), "global.archiveAuditToken")
	require.Contains(t, err.Error(), "http://dcs-orce:1880/archive/notary")
}

func TestArchiveNotaryConfigRefusesAWhitespaceToken(t *testing.T) {
	require.Error(t, ValidateArchiveNotaryConfig("http://dcs-orce:1880/archive/notary", "   "))
}

// No notary is a supported deployment: apply.go notarizes only when a client
// exists, so an instance without one archives without a receipt.
func TestArchiveNotaryConfigAcceptsNoNotaryAtAll(t *testing.T) {
	require.NoError(t, ValidateArchiveNotaryConfig("", ""))
	require.NoError(t, ValidateArchiveNotaryConfig("   ", "token"))
}

func TestArchiveNotaryConfigAcceptsAFullyConfiguredNotary(t *testing.T) {
	require.NoError(t, ValidateArchiveNotaryConfig("http://dcs-orce:1880/archive/notary", "shared-token"))
}
