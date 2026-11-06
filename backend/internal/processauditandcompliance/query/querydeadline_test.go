package qry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const deadlineContract = `{
  "dcs:policies": {
    "@type": "odrl:Agreement",
    "odrl:obligation": [{
      "@id": "urn:uuid:report-q3",
      "odrl:action": {"@id": "odrl:inform"},
      "odrl:constraint": [{
        "odrl:leftOperand": {"@id": "odrl:dateTime"},
        "odrl:operator": {"@id": "odrl:lteq"},
        "odrl:rightOperand": {"@value": "2026-09-30T23:59:59Z", "@type": "xsd:dateTime"}
      }]
    }, {
      "@id": "urn:uuid:report-2027",
      "odrl:constraint": {
        "odrl:leftOperand": "odrl:dateTime",
        "odrl:operator": "odrl:lt",
        "odrl:rightOperand": "2027-01-01"
      }
    }],
    "odrl:permission": [{
      "@id": "urn:uuid:use-until",
      "odrl:constraint": [{
        "odrl:leftOperand": {"@id": "odrl:dateTime"},
        "odrl:operator": {"@id": "odrl:lt"},
        "odrl:rightOperand": "2026-01-01"
      }],
      "odrl:duty": [{
        "@id": "urn:uuid:pay-by",
        "odrl:constraint": [{
          "odrl:leftOperand": {"@id": "odrl:dateTime"},
          "odrl:operator": {"@id": "odrl:lt"},
          "odrl:rightOperand": "2026-10-01"
        }]
      }]
    }]
  }
}`

// DCS-FR-CWE-09 (ADR-33): a duty whose deadline passed with no report from the
// target is flagged. A deadline still ahead, a rule the target reported on, and
// a time-bounded permission (use-time context, not a duty) are not.
func TestMissedDeadlineRisksFlagOnlyPassedUnreportedDuties(t *testing.T) {
	checkedAt := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	contracts := []activeContract{{DID: "did:web:c1", ContractData: []byte(deadlineContract)}}

	risks := missedDeadlineRisksFrom(contracts, map[string]time.Time{}, checkedAt)

	require.Len(t, risks, 2)
	require.Equal(t, RiskTypeDeadlineMissed, risks[0].RiskType)
	require.Equal(t, "did:web:c1", risks[0].DID)
	require.Contains(t, risks[0].Detail, "urn:uuid:report-q3")
	require.Contains(t, risks[0].Detail, "2026-09-30T23:59:59Z")
	require.Contains(t, risks[1].Detail, "urn:uuid:pay-by")

	reported := map[string]time.Time{"did:web:c1|urn:uuid:report-q3": time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)}
	risks = missedDeadlineRisksFrom(contracts, reported, checkedAt)
	require.Len(t, risks, 1)
	require.Contains(t, risks[0].Detail, "urn:uuid:pay-by")
}

// DCS-FR-CWE-09: a report that arrives after the deadline does not cover it;
// the miss stays a violation.
func TestALateReportDoesNotCoverAMissedDeadline(t *testing.T) {
	checkedAt := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	contracts := []activeContract{{DID: "did:web:c1", ContractData: []byte(deadlineContract)}}
	late := map[string]time.Time{"did:web:c1|urn:uuid:report-q3": time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)}

	risks := missedDeadlineRisksFrom(contracts, late, checkedAt)

	require.Len(t, risks, 2)
	require.Contains(t, risks[0].Detail, "urn:uuid:report-q3")
}

// The detail derives from the document only, so the register sees the same
// finding on every sweep and alerts once.
func TestMissedDeadlineRiskDetailIsStableAcrossSweeps(t *testing.T) {
	contracts := []activeContract{{DID: "did:web:c1", ContractData: []byte(deadlineContract)}}
	first := missedDeadlineRisksFrom(contracts, nil, time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC))
	second := missedDeadlineRisksFrom(contracts, nil, time.Date(2026, 10, 30, 12, 5, 0, 0, time.UTC))
	require.Equal(t, first[0].Detail, second[0].Detail)
}

// DCS-FR-SM-13: a contract still waiting for signatures after its signing
// deadline is a risk; one whose deadline lies ahead, or states none, is not.
func TestSigningDeadlineRisks(t *testing.T) {
	checkedAt := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	risks := signingDeadlineRisksFrom([]activeContract{
		{DID: "passed", ContractData: []byte(`{"dcs:signingDeadline": "2026-10-29"}`)},
		{DID: "ahead", ContractData: []byte(`{"dcs:signingDeadline": "2026-10-30"}`)},
		{DID: "none", ContractData: []byte(`{}`)},
	}, checkedAt)
	require.Len(t, risks, 1)
	require.Equal(t, "passed", risks[0].DID)
	require.Equal(t, RiskTypeSigningDeadlineMissed, risks[0].RiskType)
}
