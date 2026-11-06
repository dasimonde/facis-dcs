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

// A renewal of a running contract starts when the original ends.
func TestRenewalTermFollowsARunningTerm(t *testing.T) {
	start, exp, err := renewalTerm(day(2026, 1, 1), day(2027, 1, 1), nil, nil, false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2027, 1, 1), start)
	require.Equal(t, day(2028, 1, 1), exp)
}

// An expired original's renewal starts tomorrow, the earliest start Update
// accepts, and runs a full term.
func TestRenewalTermStartsAnExpiredTermTomorrow(t *testing.T) {
	start, exp, err := renewalTerm(day(2025, 10, 1), day(2026, 10, 1), nil, nil, false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 10, 24), start)
	require.Equal(t, day(2027, 10, 24), exp)
}

// An original that expires later today: the renewal starts tomorrow too.
func TestRenewalTermStartsNoEarlierThanTomorrow(t *testing.T) {
	origStart := renewalNow.AddDate(-1, 0, 0).Add(2 * time.Hour)
	origExp := renewalNow.Add(2 * time.Hour)
	start, _, err := renewalTerm(&origStart, &origExp, nil, nil, false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 10, 24), start)
}

// A short term that expired long ago still gets a full term, not the rest of
// the term that contains today.
func TestRenewalTermGivesALongExpiredShortTermAFullTerm(t *testing.T) {
	origStart := renewalNow.AddDate(0, 0, -59)
	origExp := renewalNow.AddDate(0, 0, -29)
	start, exp, err := renewalTerm(&origStart, &origExp, nil, nil, false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 10, 24), start)
	require.Equal(t, day(2026, 10, 24).Add(30*24*time.Hour), *exp)
}

func TestRenewalTermKeepsTheAnniversaryOfAYearlyTerm(t *testing.T) {
	start, exp, err := renewalTerm(day(2027, 2, 28), day(2028, 2, 28), nil, nil, false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2028, 2, 28), start)
	require.Equal(t, day(2029, 2, 28), exp)
}

func TestRenewalTermKeepsTheMonthEnd(t *testing.T) {
	start, exp, err := renewalTerm(day(2027, 1, 31), day(2027, 2, 28), nil, nil, false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2027, 2, 28), start)
	require.Equal(t, day(2027, 3, 28), exp)

	start, exp, err = renewalTerm(day(2026, 12, 31), day(2027, 1, 31), nil, nil, false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2027, 1, 31), start)
	require.Equal(t, day(2027, 2, 28), exp)
}

// A terminated original never reached its expiry: the renewal starts tomorrow.
func TestRenewalTermStartsATerminatedOriginalTomorrow(t *testing.T) {
	start, exp, err := renewalTerm(day(2026, 1, 1), day(2027, 1, 1), nil, nil, true, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 10, 24), start)
	require.Equal(t, day(2027, 10, 24), exp)
}

func TestRenewalTermWithoutAStartLeavesTheExpiryEmpty(t *testing.T) {
	start, exp, err := renewalTerm(nil, day(2027, 1, 1), nil, nil, false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2027, 1, 1), start)
	require.Nil(t, exp)

	start, exp, err = renewalTerm(nil, nil, nil, nil, false, renewalNow)
	require.NoError(t, err)
	require.Nil(t, start)
	require.Nil(t, exp)
}

func TestRenewalTermRunsAGivenStartForTheOriginalsTerm(t *testing.T) {
	start, exp, err := renewalTerm(day(2025, 10, 1), day(2026, 10, 1), day(2026, 11, 1), nil, false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 11, 1), start)
	require.Equal(t, day(2027, 11, 1), exp)
}

func TestRenewalTermTakesGivenDates(t *testing.T) {
	start, exp, err := renewalTerm(day(2025, 10, 1), day(2026, 10, 1), day(2026, 11, 1), day(2027, 11, 1), false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 11, 1), start)
	require.Equal(t, day(2027, 11, 1), exp)

	start, exp, err = renewalTerm(day(2025, 10, 1), day(2026, 10, 1), nil, day(2027, 11, 1), false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, day(2026, 10, 24), start)
	require.Equal(t, day(2027, 11, 1), exp)
}

// Given dates are stored in UTC, so the row and dcs:metadata agree.
func TestRenewalTermNormalizesGivenDatesToUTC(t *testing.T) {
	cest := time.FixedZone("CEST", 2*60*60)
	givenStart := time.Date(2026, 11, 1, 0, 0, 0, 0, cest)
	givenExp := time.Date(2027, 11, 1, 0, 0, 0, 0, cest)
	start, exp, err := renewalTerm(nil, nil, &givenStart, &givenExp, false, renewalNow)
	require.NoError(t, err)
	require.Equal(t, time.UTC, start.Location())
	require.Equal(t, time.UTC, exp.Location())
	require.True(t, start.Equal(givenStart))
}

func TestRenewalTermRejectsInconsistentDates(t *testing.T) {
	_, _, err := renewalTerm(nil, nil, nil, day(2026, 1, 1), false, renewalNow)
	require.ErrorIs(t, err, ErrInvalidRenewalTerm)

	_, _, err = renewalTerm(nil, nil, day(2027, 6, 1), day(2027, 1, 1), false, renewalNow)
	require.ErrorIs(t, err, ErrInvalidRenewalTerm)

	_, exp, err := renewalTerm(nil, nil, nil, day(2026, 10, 24), false, renewalNow)
	require.NoError(t, err, "tomorrow is the earliest accepted date")
	require.Equal(t, day(2026, 10, 24), exp)

	// Update refuses dates before tomorrow, so the renewal does too.
	_, _, err = renewalTerm(nil, nil, day(2026, 10, 23), day(2027, 1, 1), false, renewalNow)
	require.ErrorIs(t, err, ErrInvalidRenewalTerm)
	tonight := day(2026, 10, 23).Add(20 * time.Hour)
	_, _, err = renewalTerm(nil, nil, nil, &tonight, false, renewalNow)
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
