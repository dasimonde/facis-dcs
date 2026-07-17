package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/pidverify"
)

// ceremonyTTL is how long a started ceremony stays valid for a wallet to
// present the PID before it must be restarted.
const ceremonyTTL = 15 * time.Minute

// ceremonyAudience is the fixed OID4VP audience/client_id bound into the
// KB-JWT of a PID presentation for a signing ceremony.
const ceremonyAudience = pidverify.Audience

// WebhookSecret returns the shared secret that authenticates the EUDIPLO
// OID4VP webhook (NFR-SEC-18). It is read from EUDIPLO_WEBHOOK_SECRET.
func WebhookSecret() string {
	if v := strings.TrimSpace(os.Getenv("EUDIPLO_WEBHOOK_SECRET")); v != "" {
		return v
	}
	return "bdd-eudiplo-webhook-secret"
}

// StartCeremonyCmd carries the inputs for starting a signing ceremony.
type StartCeremonyCmd struct {
	ContractDID string
	FieldName   string
	RequestedBy string
	BaseURL     string
}

// StartCeremonyHandler creates a pending signing ceremony (FR-SM-14).
type StartCeremonyHandler struct {
	DB           *sqlx.DB
	CeremonyRepo db.CeremonyRepo
}

func buildCeremonyWalletURI(baseURL, ceremonyID string) string {
	requestURI := strings.TrimRight(baseURL, "/") + "/signature/presentation/request/" + url.PathEscape(ceremonyID)

	q := url.Values{}
	q.Set("client_id", ceremonyAudience)
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

	now := time.Now().UTC()
	id := uuid.NewString()
	nonce := uuid.NewString()
	baseURL := strings.TrimRight(strings.TrimSpace(cmd.BaseURL), "/")

	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}

	walletURI := buildCeremonyWalletURI(baseURL, id)
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

// ErrWebhookUnauthorized is returned when the webhook shared secret is missing
// or incorrect (NFR-SEC-18).
var ErrWebhookUnauthorized = errors.New("incorrect webhook shared secret")

// ErrCeremonyNotFound is returned when a webhook references an unknown ceremony.
var ErrCeremonyNotFound = errors.New("ceremony not found")

// WebhookCmd carries a completed PID presentation from EUDIPLO.
type WebhookCmd struct {
	Secret     string
	CeremonyID string
	VpToken    string
	PidClaims  any
}

// WebhookHandler validates a PID presentation and marks the ceremony verified.
type WebhookHandler struct {
	DB           *sqlx.DB
	CeremonyRepo db.CeremonyRepo
}

func (h *WebhookHandler) Handle(ctx context.Context, cmd WebhookCmd) (*db.SignatureCeremony, error) {
	if strings.TrimSpace(cmd.Secret) == "" || cmd.Secret != WebhookSecret() {
		return nil, ErrWebhookUnauthorized
	}

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

	signerDID, sdHash, err := pidverify.Verify(cmd.VpToken)
	if err != nil {
		return nil, fmt.Errorf("pid presentation verification failed: %w", err)
	}

	var pidBytes []byte
	if cmd.PidClaims != nil {
		if b, mErr := json.Marshal(cmd.PidClaims); mErr == nil {
			pidBytes = b
		}
	}

	if err := h.CeremonyRepo.MarkCeremonyVerified(ctx, tx, cmd.CeremonyID, signerDID, cmd.VpToken, pidBytes, sdHash); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit webhook: %w", err)
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
