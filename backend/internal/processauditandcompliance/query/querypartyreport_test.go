package qry

import (
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }

// DCS-FR-PACM-07: the report is segmented by party, and every party carries
// status, timestamp, non-conformances and the credential metadata.
func TestPartyReportJudgesEachPartyFromItsCredential(t *testing.T) {
	signedAt := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	credentials := []CredentialMetadata{
		{Party: "did:web:org-a", SignerDID: "did:key:jane", SignatureStatus: "SIGNED", SignedAt: &signedAt,
			AchievedLevel: "AES", RequiredLevel: "QES", CertSerial: ptr("0A1B"), PoAOrganization: ptr("did:web:org-a")},
		{Party: "did:web:org-c", SignerDID: "did:key:carl", SignatureStatus: "SIGNED", SignedAt: &signedAt,
			AchievedLevel: "AES", RequiredLevel: "AES", PoAOrganization: ptr("did:web:org-c")},
	}
	report := buildPartyReport("did:web:dcs:contract:c1", "SIGNED", []string{"did:web:org-a", "did:web:org-b"}, credentials)

	if len(report.Parties) != 3 {
		t.Fatalf("parties = %+v", report.Parties)
	}
	a, b, c := report.Parties[0], report.Parties[1], report.Parties[2]
	if a.Party != "did:web:org-a" || a.Status != StatusNonCompliant || len(a.NonConformances) != 1 ||
		a.Timestamp == nil || !a.Timestamp.Equal(signedAt) || a.Credential == nil || *a.Credential.CertSerial != "0A1B" {
		t.Errorf("party a = %+v", a)
	}
	if b.Party != "did:web:org-b" || b.Status != StatusPending || b.Credential != nil {
		t.Errorf("party b = %+v", b)
	}
	if c.Party != "did:web:org-c" || c.Status != StatusCompliant || len(c.NonConformances) != 0 {
		t.Errorf("party c = %+v", c)
	}
	if report.Status != StatusNonCompliant {
		t.Errorf("report status = %s", report.Status)
	}

	credentials[0].AchievedLevel = "QES"
	if got := buildPartyReport("did", "SIGNED", []string{"did:web:org-a"}, credentials[:1]); got.Status != StatusCompliant {
		t.Errorf("QES signature for a QES party: status %s", got.Status)
	}
	credentials[0].PoAOrganization = ptr("did:web:other")
	if got := buildPartyReport("did", "SIGNED", []string{"did:web:org-a"}, credentials[:1]); got.Status != StatusNonCompliant {
		t.Errorf("PoA for another organization: status %s", got.Status)
	}
}
