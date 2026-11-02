package command

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/signingmanagement/db"
)

type ceremoniesByID map[string]*db.SignatureCeremony

func (c ceremoniesByID) GetCeremonyByID(_ context.Context, _ *sqlx.Tx, id string) (*db.SignatureCeremony, error) {
	return c[id], nil
}

func policySignature(signer, field, level, ceremony string) db.SignatureRecord {
	return db.SignatureRecord{SignerDID: signer, Status: "SIGNED", CredentialType: level, FieldName: &field, CeremonyID: &ceremony}
}

func poaCeremony(id, presentation, roles string) *db.SignatureCeremony {
	pid := "pid-valid"
	return &db.SignatureCeremony{ID: id, PoAVpToken: &presentation, PoARoles: []byte(roles), VpToken: &pid}
}

// DCS-FR-SM-21: each signature is judged against the level the contract
// requires, the status of its certificate, of its PID and of its Power of
// Attorney, and the roles that Power of Attorney grants. A missing Power of
// Attorney and a status that cannot be determined are findings too.
func TestSignaturePolicyFindingsFlagEachViolation(t *testing.T) {
	contract := []byte(`{"dcs:signatureFields":[
		{"dcs:signatoryName":"qes-field","dcs:requiredCredentialType":"QES"},
		{"dcs:signatoryName":"aes-field"}]}`)
	revokedAt := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	compliant := policySignature("did:example:ok", "qes-field", "QES", "c-ok")
	belowLevel := policySignature("did:example:low", "qes-field", "AES", "c-low")
	revokedCert := policySignature("did:example:cert", "aes-field", "AES", "c-cert")
	revokedCert.CertRevokedAt = &revokedAt
	noSigningRole := policySignature("did:example:role", "aes-field", "AES", "c-role")
	revokedPoA := policySignature("did:example:poa", "aes-field", "AES", "c-poa")
	unreadablePoA := policySignature("did:example:unknown", "aes-field", "AES", "c-unknown")
	noPoA := policySignature("did:example:nopoa", "aes-field", "AES", "c-nopoa")
	revokedPID := policySignature("did:example:pid", "aes-field", "AES", "c-pid")
	revokedSignature := policySignature("did:example:gone", "qes-field", "AES", "c-gone")
	revokedSignature.Status = "REVOKED"

	ceremonies := ceremoniesByID{
		"c-ok":      poaCeremony("c-ok", "valid", `["Contract Signer"]`),
		"c-low":     poaCeremony("c-low", "valid", `["Contract Signer"]`),
		"c-cert":    poaCeremony("c-cert", "valid", `["Contract Signer"]`),
		"c-role":    poaCeremony("c-role", "valid", `["Contract Observer"]`),
		"c-poa":     poaCeremony("c-poa", "revoked", `["Contract Signer"]`),
		"c-unknown": poaCeremony("c-unknown", "unreadable", `["Contract Signer"]`),
		"c-nopoa":   {ID: "c-nopoa"},
		"c-pid":     poaCeremony("c-pid", "valid", `["Contract Signer"]`),
	}
	revokedPIDPresentation := "revoked"
	ceremonies["c-pid"].VpToken = &revokedPIDPresentation
	state := func(_ context.Context, presentation string) (status.State, error) {
		switch presentation {
		case "revoked":
			return status.StateInvalid, nil
		case "unreadable":
			return "", errors.New("status list unreachable")
		}
		return status.StateValid, nil
	}

	findings, err := signaturePolicyFindings(context.Background(), nil, ceremonies, state, state, contract,
		[]db.SignatureRecord{compliant, belowLevel, revokedCert, noSigningRole, revokedPoA, unreadablePoA, noPoA, revokedPID, revokedSignature})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`Signer did:example:low signed "qes-field" at level AES, the contract requires QES`,
		"Signer did:example:cert signed with a certificate revoked at 2026-10-01T00:00:00Z",
		"The Power of Attorney presented by signer did:example:role grants no signing role",
		"The Power of Attorney presented by signer did:example:poa is invalid",
		"The credential status of signer did:example:unknown could not be determined: Power of Attorney: status list unreachable",
		"The signature of signer did:example:nopoa carries no Power of Attorney presentation",
		"The PID presented by signer did:example:pid is invalid",
	}
	if strings.Join(findings, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(findings, "\n"), strings.Join(want, "\n"))
	}
}

func TestComplianceDropsThePassedFindingWhenAnotherFlags(t *testing.T) {
	if got := withoutPassedWhenFlagged([]string{"Compliance checks passed"}); len(got) != 1 {
		t.Errorf("a clean check keeps its passed finding, got %v", got)
	}
	got := withoutPassedWhenFlagged([]string{"Compliance checks passed", "Signer x grants no signing role"})
	if len(got) != 1 || got[0] != "Signer x grants no signing role" {
		t.Errorf("a flagged check must not also read passed, got %v", got)
	}
}
