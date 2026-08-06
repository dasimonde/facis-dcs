package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// Ceremony lifecycle statuses (FR-SM-14). A signature may only be applied once
// a ceremony for the signer+contract has reached CeremonyVerified.
const (
	CeremonyPending  = "pending"
	CeremonyVerified = "verified"
	CeremonyExpired  = "expired"
	CeremonyFailed   = "failed"
)

// SignatureCeremony is a signing ceremony: a request for the signer's wallet to
// present a PID over OID4VP directly (ADR-20; EUDIPLO is not a dependency)
// that must complete before a PAdES signature can be applied (FR-SM-14,
// UC-04-02).
type SignatureCeremony struct {
	ID          string  `db:"id"`
	ContractDID string  `db:"contract_did"`
	FieldName   string  `db:"field_name"`
	RequestedBy string  `db:"requested_by"`
	Status      string  `db:"status"`
	WalletURI   *string `db:"wallet_uri"`
	Nonce       string  `db:"nonce"`
	SignerDID   *string `db:"signer_did"`
	VpToken     *string `db:"vp_token"`
	PidClaims   []byte  `db:"pid_claims"`
	KbSdHash    *string `db:"kb_sd_hash"`
	// The Power of Attorney presented at signing (UC-14, FR-SM-03): the verified
	// organization the signatory is authorized to act for, their roles, and the
	// presentation itself as the wallet delivered it — the evidence a
	// counterparty needs to verify this signature's authority for itself
	// (ADR-31). All nil until the ceremony's PoA presentation is verified.
	PoAOrganization *string    `db:"poa_organization"`
	PoARoles        []byte     `db:"poa_roles"`
	PoAVpToken      *string    `db:"poa_vp_token"`
	CreatedAt       time.Time  `db:"created_at"`
	VerifiedAt      *time.Time `db:"verified_at"`
	ExpiresAt       time.Time  `db:"expires_at"`
	// The published OID4VP Document-Retrieval signing request (ADR-12): the
	// to-be-signed PDF and its digest the wallet fetches and signs, the request
	// object's nonce/expiry, and the publishing signer's participant context the
	// JWT-less callback replays into finalize. All nil until publish.
	PreparedPDF        []byte     `db:"prepared_pdf"`
	PreparedPDFSHA256  *string    `db:"prepared_pdf_sha256"`
	RequestNonce       *string    `db:"request_nonce"`
	RequestExpiresAt   *time.Time `db:"request_expires_at"`
	CredentialType     *string    `db:"credential_type"`
	PublishedBy        *string    `db:"published_by"`
	PublishedHolderDID *string    `db:"published_holder_did"`
	PublishedRoles     []byte     `db:"published_roles"`
	ConsumedAt         *time.Time `db:"consumed_at"`

	// Pinned at EVERY prepare (wallet ceremony and desktop /signature/prepare
	// alike, ADR-20): the exact to-be-signed material and the finalize
	// metadata derived alongside it, so submit validates against committed
	// bytes instead of re-deriving them, and applies no side effects.
	PinnedPayload         []byte  `db:"pinned_payload"`
	PinnedPayloadSHA256   *string `db:"pinned_payload_sha256"`
	PinnedContentHash     *string `db:"pinned_content_hash"`
	PinnedRendererVersion *string `db:"pinned_renderer_version"`
	PinnedSignedCount     *int    `db:"pinned_signed_count"`
	PinnedContractVersion *int    `db:"pinned_contract_version"`
	// RequiredCredentialType is the contract's OWN declared signature-level
	// requirement for this field (dcs:requiredCredentialType, default AES),
	// pinned at prepare so submit gates on it rather than on the caller-
	// supplied credential_type (SM-01 per-contract level enforcement).
	RequiredCredentialType *string `db:"required_credential_type"`

	// SignerCertSubject/SignerCertSerial are the signing certificate's subject
	// and serial (eIDAS Art. 26c sole control), recorded once a signature
	// validates, for the Signature Compliance Viewer and cross-ceremony
	// consistency checks (DCS-FR-SM-26).
	SignerCertSubject *string `db:"signer_cert_subject"`
	SignerCertSerial  *string `db:"signer_cert_serial"`
}

// PinnedBytes is the exact to-be-signed material and derived metadata pinned
// at prepare (ADR-20): the base PDF (already stored via PreparedPDF/
// PreparedPDFSHA256, shared with the publish flow), the canonical JAdES
// payload, and the finalize inputs that must not be re-derived at submit.
type PinnedBytes struct {
	CeremonyID             string
	PreparedPDF            []byte
	PreparedPDFSHA256      string
	PinnedPayload          []byte
	PinnedPayloadSHA256    string
	PinnedContentHash      string
	PinnedRendererVersion  string
	PinnedSignedCount      int
	PinnedContractVersion  int
	RequiredCredentialType string
}

// PreparedRequest carries the published OID4VP signing request state persisted on
// a ceremony at publish (ADR-12).
type PreparedRequest struct {
	CeremonyID        string
	PreparedPDF       []byte
	PreparedPDFSHA256 string
	RequestNonce      string
	RequestExpiresAt  time.Time
	CredentialType    string
	PublishedBy       string
	HolderDID         string
	Roles             []byte
}

// VerifiedPresentation is the outcome of a completed signing-ceremony
// presentation: the verified signatory, the PID presentation and its disclosed
// claims, and the Power of Attorney that authorized the signature.
type VerifiedPresentation struct {
	CeremonyID      string
	SignerDID       string
	VpToken         string
	PidClaims       []byte
	KbSdHash        string
	PoAOrganization string
	PoARoles        []byte
	PoAVpToken      string
}

// CeremonyRepo persists signing ceremonies.
type CeremonyRepo interface {
	CreateCeremony(ctx context.Context, tx *sqlx.Tx, c SignatureCeremony) error
	GetCeremonyByID(ctx context.Context, tx *sqlx.Tx, id string) (*SignatureCeremony, error)
	MarkCeremonyVerified(ctx context.Context, tx *sqlx.Tx, verified VerifiedPresentation) error
	// RecordSummaryVC retains the signing summary issued for a ceremony.
	RecordSummaryVC(ctx context.Context, tx *sqlx.Tx, ceremonyID string, summary []byte) error
	// StorePreparedRequest persists the published signing request (the
	// to-be-signed PDF + digest + request object nonce/expiry + the publishing
	// signer's context) on a verified ceremony (ADR-12 publish).
	StorePreparedRequest(ctx context.Context, tx *sqlx.Tx, req PreparedRequest) error
	// MarkCeremonyConsumed records that the signed document has been accepted at
	// the callback, so a published request is single-use. The caller runs it in
	// the SAME transaction as SubmitSignature's finalize (ADR-20 atomic
	// consumption): the UPDATE ... WHERE consumed_at IS NULL guard and the
	// finalize writes commit or roll back together, so two concurrent
	// callbacks can never both finalize.
	MarkCeremonyConsumed(ctx context.Context, tx *sqlx.Tx, id string) error
	// PinPreparedBytes persists the exact to-be-signed material (PDF + JAdES
	// payload) and the finalize metadata derived alongside it, at every
	// prepare (ADR-20). A later prepare on the same ceremony overwrites the
	// pin with fresh bytes.
	PinPreparedBytes(ctx context.Context, tx *sqlx.Tx, pinned PinnedBytes) error
	// RecordSignerCertificate persists the validated signing certificate's
	// subject and serial on the ceremony (sole control evidence, DCS-FR-SM-26).
	RecordSignerCertificate(ctx context.Context, tx *sqlx.Tx, ceremonyID, subject, serial string) error
	// FindVerifiedCeremony returns the most recent verified ceremony for the
	// given contract and signer, or (nil, nil) when none exists.
	FindVerifiedCeremony(ctx context.Context, tx *sqlx.Tx, contractDID, signerDID string) (*SignatureCeremony, error)
	// FindVerifiedCeremonyByField returns the most recent verified ceremony
	// for the given contract and signature FIELD, or (nil, nil) when none
	// exists — the all-ceremonies-before-first-signature gate of the
	// multi-signer flow (DCS-FR-SM-07/-17) checks every declared field.
	FindVerifiedCeremonyByField(ctx context.Context, tx *sqlx.Tx, contractDID, fieldName string) (*SignatureCeremony, error)
}
