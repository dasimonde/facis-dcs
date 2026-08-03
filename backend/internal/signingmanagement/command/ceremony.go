package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/signingmanagement/db"
)

// ceremonyTTL is how long a started ceremony stays valid for a wallet to
// present the PID before it must be restarted.
const ceremonyTTL = 15 * time.Minute

// StartCeremonyCmd carries the inputs for starting a signing ceremony.
type StartCeremonyCmd struct {
	ContractDID string
	FieldName   string
	RequestedBy string
	BaseURL     string
	// ClientID is the OpenID4VP client identifier this deployment presents
	// itself to a wallet with — the x509_san_dns identifier its own certificate
	// backs. It is the audience the presented KB-JWTs bind to, so it travels
	// into the deep link the wallet scans rather than being a fixed name.
	ClientID string
}

// StartCeremonyHandler creates a pending signing ceremony (FR-SM-14).
type StartCeremonyHandler struct {
	DB           *sqlx.DB
	CeremonyRepo db.CeremonyRepo
}

func buildCeremonyWalletURI(baseURL, ceremonyID, clientID string) string {
	requestURI := strings.TrimRight(baseURL, "/") + "/signature/request/" + url.PathEscape(ceremonyID) + "/object"

	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("request_uri", requestURI)
	q.Set("request_uri_method", "post")

	return "openid4vp://?" + q.Encode()
}

func (h *StartCeremonyHandler) Handle(ctx context.Context, cmd StartCeremonyCmd) (*db.SignatureCeremony, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer rollback(tx)

	if strings.TrimSpace(cmd.ClientID) == "" {
		return nil, fmt.Errorf("openid4vp client_id is not configured")
	}

	now := time.Now().UTC()
	id := uuid.NewString()
	nonce := uuid.NewString()
	walletURI := buildCeremonyWalletURI(cmd.BaseURL, id, cmd.ClientID)
	expiresAt := now.Add(ceremonyTTL)

	ceremony := db.SignatureCeremony{
		ID:          id,
		ContractDID: cmd.ContractDID,
		FieldName:   cmd.FieldName,
		RequestedBy: cmd.RequestedBy,
		Status:      db.CeremonyPending,
		WalletURI:   &walletURI,
		Nonce:       nonce,
		CreatedAt:   now,
		ExpiresAt:   expiresAt,
	}
	if err := h.CeremonyRepo.CreateCeremony(ctx, tx, ceremony); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit start ceremony: %w", err)
	}
	return &ceremony, nil
}

// ErrCeremonyNotFound is returned when the presentation references an unknown
// ceremony.
var ErrCeremonyNotFound = errors.New("ceremony not found")

// ErrCeremonyExpired rejects a presentation that arrives after the ceremony's
// deadline (DCS-FR-SM-13): the signing workflow enforces the deadline issued
// at ceremony start (expires_at, ceremonyTTL) — a late wallet presentation
// does not verify, and the signer must request a fresh ceremony (the
// workflow's retry).
var ErrCeremonyExpired = errors.New("ceremony deadline has passed; request a new signing ceremony")

// ErrPoAUnauthorized is returned when the signing presentation carries no Power
// of Attorney, or a PoA that authorizes a different organization than the party
// (signature field) being signed — signing is not authorized (UC-14, FR-SM-03).
var ErrPoAUnauthorized = errors.New("power of attorney does not authorize this signature")

// PresentationCmd carries a completed signing-ceremony presentation, ALREADY
// cryptographically verified against the ceremony's own nonce and the
// configured trust anchors by the caller (oid4vp.Verifier.VerifyPID, ADR-20):
// the resolved signatory DID and SD-JWT key-binding hash, the disclosed PID
// claims, and the Power of Attorney presented at signing (UC-14, FR-SM-03),
// whose organization the signature is checked against. CompletePresentation
// persists this outcome; it performs no verification of its own.
type PresentationCmd struct {
	CeremonyID      string
	SignerDID       string
	SDHash          string
	VpToken         string
	PidClaims       any
	PoAOrganization string
	PoARoles        any
	// PoAVpToken is the Power-of-Attorney presentation as the wallet delivered
	// it, retained so the counterparty can verify this signature's authority
	// for itself rather than reading an unbacked claim (ADR-31).
	PoAVpToken string
}

// PresentationHandler records a verified PID+PoA presentation and marks the
// ceremony verified. The caller (SignatureRequestCallback's direct_post
// vp_token branch) has already cryptographically verified the presentation
// against the ceremony's nonce and the configured trust anchors before
// calling this — CompletePresentation only persists the outcome.
type PresentationHandler struct {
	DB           *sqlx.DB
	CeremonyRepo db.CeremonyRepo
}

func (h *PresentationHandler) CompletePresentation(ctx context.Context, cmd PresentationCmd) (*db.SignatureCeremony, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer rollback(tx)

	ceremony, err := h.CeremonyRepo.GetCeremonyByID(ctx, tx, cmd.CeremonyID)
	if err != nil {
		return nil, err
	}
	if ceremony == nil {
		return nil, ErrCeremonyNotFound
	}
	if time.Now().UTC().After(ceremony.ExpiresAt) {
		return nil, fmt.Errorf("%w (expired %s)", ErrCeremonyExpired, ceremony.ExpiresAt.UTC().Format(time.RFC3339))
	}

	if strings.TrimSpace(cmd.SignerDID) == "" {
		return nil, fmt.Errorf("presentation carries no verified signatory DID")
	}
	signerDID, sdHash := cmd.SignerDID, cmd.SDHash

	var pidBytes []byte
	if cmd.PidClaims != nil {
		if b, mErr := json.Marshal(cmd.PidClaims); mErr == nil {
			pidBytes = b
		}
	}

	// The Power of Attorney presented at signing authorizes the signatory to act
	// for its organization. UC-14 requires a valid PoA BEFORE a contract can be
	// signed and only "then authorizes the signing operation", so a missing PoA is
	// a hard failure here: the ceremony does not verify and signing cannot proceed.
	// It must also authorize the party actually signed — the signature field is the
	// participating org DID (SeedSignatureFields), so the PoA organization must
	// equal the ceremony's field (FR-SM-03).
	poaOrganization := strings.TrimSpace(cmd.PoAOrganization)
	if poaOrganization == "" {
		return nil, fmt.Errorf("%w: no Power of Attorney credential was presented at signing", ErrPoAUnauthorized)
	}
	if poaOrganization != ceremony.FieldName {
		return nil, fmt.Errorf("%w: Power of Attorney authorizes %q, not the signed party %q", ErrPoAUnauthorized, poaOrganization, ceremony.FieldName)
	}
	var poaRoles []byte
	if cmd.PoARoles != nil {
		if b, mErr := json.Marshal(cmd.PoARoles); mErr == nil {
			poaRoles = b
		}
	}

	if err := h.CeremonyRepo.MarkCeremonyVerified(ctx, tx, db.VerifiedPresentation{
		CeremonyID:      cmd.CeremonyID,
		SignerDID:       signerDID,
		VpToken:         cmd.VpToken,
		PidClaims:       pidBytes,
		KbSdHash:        sdHash,
		PoAOrganization: poaOrganization,
		PoARoles:        poaRoles,
		PoAVpToken:      strings.TrimSpace(cmd.PoAVpToken),
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit presentation: %w", err)
	}

	ceremony.Status = db.CeremonyVerified
	ceremony.SignerDID = &signerDID
	return ceremony, nil
}

func rollback(tx *sqlx.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		log.Printf("could not rollback transaction: %v", err)
	}
}
