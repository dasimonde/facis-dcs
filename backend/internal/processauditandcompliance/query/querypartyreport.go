package qry

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
)

// ErrContractNotFound is returned when the contract of a report does not exist.
var ErrContractNotFound = errors.New("contract not found")

const (
	StatusCompliant    = "COMPLIANT"
	StatusNonCompliant = "NON_COMPLIANT"
	StatusPending      = "PENDING"
)

// PartyReport is the compliance report of one contract segmented by party
// (DCS-FR-PACM-07). The segmentation by clause is GET /signature/clause-report.
// A party is COMPLIANT when its signature passes the checks named in
// PartyReportChecks, not because it is signed.
type PartyReport struct {
	ContractDID   string            `json:"contract_did"`
	ContractState string            `json:"contract_state"`
	GeneratedAt   time.Time         `json:"generated_at"`
	GeneratedBy   string            `json:"generated_by"`
	Checks        []string          `json:"checks"`
	Status        string            `json:"status"`
	Parties       []PartyCompliance `json:"parties"`
}

// PartyReportChecks are the checks a party's signature is judged by.
var PartyReportChecks = []string{
	"signature not revoked",
	"achieved signature level meets the required level",
	"signing certificate not revoked",
	"Power of Attorney authorizes the party",
}

// CredentialMetadata describes the credential behind one signature.
type CredentialMetadata struct {
	Party           string     `json:"party" db:"party"`
	SignerDID       string     `json:"signer_did" db:"signer_did"`
	SignatureStatus string     `json:"signature_status" db:"signature_status"`
	SignedAt        *time.Time `json:"signed_at,omitempty" db:"signed_at"`
	AchievedLevel   string     `json:"achieved_level" db:"achieved_level"`
	RequiredLevel   string     `json:"required_level" db:"required_level"`
	CertSubject     *string    `json:"signer_cert_subject,omitempty" db:"signer_cert_subject"`
	CertSerial      *string    `json:"signer_cert_serial,omitempty" db:"signer_cert_serial"`
	CertRevokedAt   *time.Time `json:"cert_revoked_at,omitempty" db:"cert_revoked_at"`
	PoAOrganization *string    `json:"poa_organization,omitempty" db:"poa_organization"`
	KbSdHash        *string    `json:"kb_sd_hash,omitempty" db:"kb_sd_hash"`
	// Source is "peer" for a signature a counterparty applied on its own
	// instance. Its level is judged there (ADR-37) and not recorded here.
	Source string `json:"source" db:"-"`
}

type PartyCompliance struct {
	Party           string              `json:"party"`
	Status          string              `json:"status"`
	Timestamp       *time.Time          `json:"timestamp,omitempty"`
	NonConformances []string            `json:"non_conformances"`
	Credential      *CredentialMetadata `json:"credential,omitempty"`
}

// ContractCredentials returns the credential metadata of every signature of
// the contract, oldest first: the signatures this instance applied, then the
// signatures the counterparties applied on their instances.
func ContractCredentials(ctx context.Context, db sqlx.QueryerContext, did string) ([]CredentialMetadata, error) {
	var rows []struct {
		CredentialMetadata
		ContractData []byte `db:"contract_data"`
	}
	if err := sqlx.SelectContext(ctx, db, &rows, `
        SELECT COALESCE(s.field_name, '') AS party, s.signer_did, s.status::text AS signature_status,
               s.signed_at, s.credential_type AS achieved_level, s.cert_revoked_at,
               c.signer_cert_subject, c.signer_cert_serial, c.poa_organization, c.kb_sd_hash,
               k.contract_data
        FROM contract_signatures s
        JOIN contracts k ON k.did = s.contract_did
        LEFT JOIN signature_ceremonies c ON c.id = s.ceremony_id
        WHERE s.contract_did = $1 ORDER BY s.id`, did); err != nil {
		return nil, fmt.Errorf("could not read signatures: %w", err)
	}
	credentials := make([]CredentialMetadata, 0, len(rows))
	for _, row := range rows {
		row.RequiredLevel = validation.RequiredCredentialType(row.ContractData, row.Party)
		row.Source = "local"
		credentials = append(credentials, row.CredentialMetadata)
	}
	peers, err := peerCredentials(ctx, db, did)
	if err != nil {
		return nil, err
	}
	return append(credentials, peers...), nil
}

// peerCredentials reads the peer signatures recorded when the counterparty's
// signed PDF arrived (contract_sync_signatures.poa_evidence).
func peerCredentials(ctx context.Context, db sqlx.QueryerContext, did string) ([]CredentialMetadata, error) {
	var rows []struct {
		Evidence     []byte    `db:"poa_evidence"`
		ReceivedAt   time.Time `db:"received_at"`
		ContractData []byte    `db:"contract_data"`
	}
	if err := sqlx.SelectContext(ctx, db, &rows, `
        SELECT s.poa_evidence, s.received_at, k.contract_data
        FROM contract_sync_signatures s JOIN contracts k ON k.did = s.did
        WHERE s.did = $1 AND s.poa_evidence IS NOT NULL`, did); err != nil {
		return nil, fmt.Errorf("could not read peer signatures: %w", err)
	}
	var credentials []CredentialMetadata
	for _, row := range rows {
		var signatures []struct {
			Party           string `json:"party"`
			Signatory       string `json:"signatory"`
			PoAOrganization string `json:"poa_organization"`
		}
		if err := json.Unmarshal(row.Evidence, &signatures); err != nil {
			return nil, fmt.Errorf("could not decode peer signatures: %w", err)
		}
		for _, signature := range signatures {
			receivedAt := row.ReceivedAt
			credential := CredentialMetadata{
				Party: signature.Party, SignerDID: signature.Signatory, SignatureStatus: "SIGNED", SignedAt: &receivedAt,
				RequiredLevel: validation.RequiredCredentialType(row.ContractData, signature.Party), Source: "peer",
			}
			if signature.PoAOrganization != "" {
				organization := signature.PoAOrganization
				credential.PoAOrganization = &organization
			}
			credentials = append(credentials, credential)
		}
	}
	return credentials, nil
}

type PartyReportQry struct {
	DID         string
	GeneratedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type PartyReporter struct {
	DB *sqlx.DB
}

// Handle returns the party report as JSON and records its generation in the
// contract's audit trail.
func (h *PartyReporter) Handle(ctx context.Context, query PartyReportQry) ([]byte, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var contract struct {
		State string `db:"state"`
		Data  []byte `db:"contract_data"`
	}
	err = tx.GetContext(ctx, &contract, `SELECT state::text AS state, contract_data FROM contracts_effective WHERE did = $1`, query.DID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrContractNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("could not read contract: %w", err)
	}
	credentials, err := ContractCredentials(ctx, tx, query.DID)
	if err != nil {
		return nil, err
	}
	report := buildPartyReport(query.DID, contract.State, validation.RequiredSignatureFields(contract.Data), credentials)
	report.GeneratedAt = time.Now().UTC()
	report.GeneratedBy = query.GeneratedBy
	content, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("could not encode party report: %w", err)
	}
	sum := sha256.Sum256(content)
	if err := event.Create(ctx, tx, event2.PartyReportGeneratedEvent{
		DID: query.DID, GeneratedBy: query.GeneratedBy, Status: report.Status,
		ContentHash: "sha256:" + hex.EncodeToString(sum[:]), OccurredAt: report.GeneratedAt,
		HolderDID: query.HolderDID, UserRoles: query.UserRoles,
	}, componenttype.ProcessAuditAndCompliance); err != nil {
		return nil, fmt.Errorf("could not record party report: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}
	return content, nil
}

// buildPartyReport judges every declared signature field and every party a
// signature names. The newest signature of a party counts: a re-signature
// follows a revocation.
func buildPartyReport(did, state string, declared []string, credentials []CredentialMetadata) PartyReport {
	report := PartyReport{ContractDID: did, ContractState: state, Checks: PartyReportChecks, Status: StatusCompliant, Parties: []PartyCompliance{}}
	parties := append([]string{}, declared...)
	known := map[string]bool{}
	for _, party := range parties {
		known[party] = true
	}
	for _, credential := range credentials {
		if !known[credential.Party] {
			known[credential.Party] = true
			parties = append(parties, credential.Party)
		}
	}
	for _, party := range parties {
		entry := PartyCompliance{Party: party, Status: StatusPending, NonConformances: []string{}}
		for i := range credentials {
			if credentials[i].Party != party {
				continue
			}
			entry.Credential = &credentials[i]
			entry.Timestamp = credentials[i].SignedAt
			entry.NonConformances = nonConformances(credentials[i])
			entry.Status = StatusCompliant
			if len(entry.NonConformances) > 0 {
				entry.Status = StatusNonCompliant
			}
		}
		if entry.Status == StatusNonCompliant || (entry.Status == StatusPending && report.Status == StatusCompliant) {
			report.Status = entry.Status
		}
		report.Parties = append(report.Parties, entry)
	}
	return report
}

func nonConformances(credential CredentialMetadata) []string {
	found := []string{}
	if credential.SignatureStatus == "REVOKED" {
		found = append(found, "the signature is revoked")
	}
	if credential.Source != "peer" && !validation.SignatureLevelSatisfies(credential.AchievedLevel, credential.RequiredLevel) {
		found = append(found, fmt.Sprintf("the signature achieved %s, the contract requires %s", credential.AchievedLevel, credential.RequiredLevel))
	}
	if credential.CertRevokedAt != nil {
		found = append(found, "the signing certificate is revoked")
	}
	if credential.PoAOrganization != nil && strings.TrimSpace(*credential.PoAOrganization) != credential.Party {
		found = append(found, fmt.Sprintf("the Power of Attorney authorizes %s, not this party", *credential.PoAOrganization))
	}
	return found
}
