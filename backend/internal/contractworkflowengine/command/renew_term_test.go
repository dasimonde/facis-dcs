package command

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
)

func day(y int, m time.Month, d int) *time.Time {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return &t
}

var renewalNow = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

// A renewal of a running contract keeps its term.
func TestRenewalTermCopiesARunningTerm(t *testing.T) {
	start, exp, err := renewalTerm(day(2026, 1, 1), day(2027, 1, 1), nil, nil, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 1, 1), start)
	require.Equal(t, day(2027, 1, 1), exp)
}

// A copied past expiry makes the new DRAFT read EXPIRED at once, so the term
// moves forward by whole terms of the original's length.
func TestRenewalTermMovesAnExpiredTermForward(t *testing.T) {
	start, exp, err := renewalTerm(day(2025, 10, 1), day(2026, 10, 1), nil, nil, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 10, 1), start)
	require.Equal(t, day(2027, 10, 1), exp)
}

func TestRenewalTermSkipsTermsThatAlsoLieInThePast(t *testing.T) {
	start, exp, err := renewalTerm(day(2026, 1, 1), day(2026, 4, 1), nil, nil, renewalNow)
	require.NoError(t, err)
	length := day(2026, 4, 1).Sub(*day(2026, 1, 1))
	require.Equal(t, day(2026, 4, 1).Add(2*length), *start)
	require.Equal(t, start.Add(length), *exp)
	require.True(t, start.Before(renewalNow) || start.Equal(renewalNow))
	require.True(t, exp.After(renewalNow))
}

func TestRenewalTermWithoutAStartLeavesTheExpiryEmpty(t *testing.T) {
	start, exp, err := renewalTerm(nil, day(2026, 10, 1), nil, nil, renewalNow)
	require.NoError(t, err)
	require.Nil(t, start)
	require.Nil(t, exp)
}

func TestRenewalTermKeepsAGivenStartForAnExpiredTerm(t *testing.T) {
	start, exp, err := renewalTerm(day(2025, 10, 1), day(2026, 10, 1), day(2026, 11, 1), nil, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 11, 1), start)
	require.Equal(t, day(2026, 11, 1).Add(day(2026, 10, 1).Sub(*day(2025, 10, 1))), *exp)
}

func TestRenewalTermTakesGivenDates(t *testing.T) {
	start, exp, err := renewalTerm(day(2025, 10, 1), day(2026, 10, 1), day(2026, 11, 1), day(2027, 11, 1), renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 11, 1), start)
	require.Equal(t, day(2027, 11, 1), exp)
}

func TestRenewalTermRejectsInconsistentDates(t *testing.T) {
	_, _, err := renewalTerm(nil, nil, nil, day(2026, 1, 1), renewalNow)
	require.ErrorIs(t, err, ErrInvalidRenewalTerm)

	_, _, err = renewalTerm(nil, nil, day(2027, 6, 1), day(2027, 1, 1), renewalNow)
	require.ErrorIs(t, err, ErrInvalidRenewalTerm)
}

func renewalMetadata(t *testing.T, payload *datatype.JSON) map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(*payload), &doc))
	meta, _ := doc["dcs:metadata"].(map[string]any)
	return meta
}

// The copy carries the original's term in dcs:metadata, which the PDF embeds
// and the peer reads.
func TestStampRenewalTermReplacesTheOriginalsTerm(t *testing.T) {
	copied := datatype.JSON(`{"dcs:metadata":{"dcs:startDate":"2025-10-01T00:00:00Z","dcs:expirationDate":"2026-10-01T00:00:00Z"}}`)

	stamped, err := stampRenewalTerm(&copied, sharedContractDetails{StartDate: day(2026, 10, 1), ExpDate: day(2027, 10, 1)})
	require.NoError(t, err)
	meta := renewalMetadata(t, stamped)
	require.Equal(t, "2026-10-01T00:00:00Z", meta[metaStartDate].(map[string]any)["@value"])
	require.Equal(t, "2027-10-01T00:00:00Z", meta[metaExpirationDate].(map[string]any)["@value"])

	stamped, err = stampRenewalTerm(&copied, sharedContractDetails{})
	require.NoError(t, err)
	meta = renewalMetadata(t, stamped)
	require.NotContains(t, meta, metaStartDate)
	require.NotContains(t, meta, metaExpirationDate)
}
