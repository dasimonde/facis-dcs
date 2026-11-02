package dcstodcs

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/jmoiron/sqlx"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	dcstodcsc "digital-contracting-service/gen/http/dcs_to_dcs/client"
	"digital-contracting-service/internal/base/identity"
)

// PeerOp is one peer endpoint: the goa method name and its design route. The
// route has no DCS_API_PATH and no did:web path prefix, so the sender and the
// receiver write it the same way.
type PeerOp struct {
	Name  string
	Route string
}

// The peer endpoints a challenge can be signed for (ADR-13).
var (
	OpPostPdf                  = PeerOp{"post_pdf", "POST " + dcstodcsc.PostPdfDcsToDcsPath()}
	OpPostSettlement           = PeerOp{"post_settlement", "POST " + dcstodcsc.PostSettlementDcsToDcsPath()}
	OpPostSettlementWithdrawal = PeerOp{"post_settlement_withdrawal", "POST " + dcstodcsc.PostSettlementWithdrawalDcsToDcsPath()}
	OpPostNegotiation          = PeerOp{"post_negotiation", "POST " + dcstodcsc.PostNegotiationDcsToDcsPath()}
	OpErase                    = PeerOp{"erase", "POST " + dcstodcsc.EraseDcsToDcsPath()}
	OpGetContractInfo          = PeerOp{"get_contract_info", "POST " + dcstodcsc.GetContractInfoDcsToDcsPath()}
	OpTerminate                = PeerOp{"terminate", "POST " + dcstodcsc.TerminateDcsToDcsPath()}
)

const peerChallengeVersion = "dcs-peer-v1"

// PeerChallengeWindow is the largest difference between the iat of a peer
// challenge and the clock of the receiver. Both instances need NTP.
const PeerChallengeWindow = 120 * time.Second

// ErrPeerChallengeRefused wraps every refusal of a peer challenge. Other
// errors of VerifyPeerRequest are failures of this instance.
var ErrPeerChallengeRefused = errors.New("peer challenge refused")

// SignPeerChallenge builds the secret_value of one peer request and signs it
// with the sender's DID key. The signed string is
//
//	<op>|<contract_iri>|dcs-peer-v1|<method> <route>|<from_did>|<to_did>|<iat>|<nonce>|<body sha256 hex or "">
//
// so a signature is valid for one operation on one contract, from one sender
// to one receiver, once, and for the body it was made for.
func SignPeerChallenge(document *identity.DIDDocument, op PeerOp, fromDID, toDID, contractIRI string, body []byte) (string, []byte, error) {
	fields := []string{op.Name, contractIRI, peerChallengeVersion, op.Route, fromDID, toDID,
		strconv.FormatInt(time.Now().Unix(), 10), rand.Text(), bodyDigest(body)}
	for _, field := range fields {
		if strings.Contains(field, "|") {
			return "", nil, fmt.Errorf("peer challenge: field %q contains the separator |", field)
		}
	}
	value := strings.Join(fields, "|")
	signature, err := document.Sign([]byte(value))
	if err != nil {
		return "", nil, err
	}
	return value, signature, nil
}

func bodyDigest(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// NegotiationMessageBody is the body a post_negotiation challenge binds: the
// RFC 8785 form of the message without its challenge fields.
func NegotiationMessageBody(req *dcstodcs.DCSToDCSNegotiationMessageRequest) ([]byte, error) {
	message := *req
	message.SecretValue, message.SecretHash = "", nil
	encoded, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(encoded)
}

// PeerNonces records the nonces of the peer challenges this instance accepted.
type PeerNonces interface {
	// Use records the nonce of fromDID until expiresAt. It returns false when
	// the nonce is already recorded.
	Use(ctx context.Context, fromDID, nonce string, expiresAt time.Time) (bool, error)
}

// DBPeerNonces keeps the nonces in Postgres, so every replica refuses a
// challenge that another replica accepted.
type DBPeerNonces struct {
	DB *sqlx.DB
}

func (n DBPeerNonces) Use(ctx context.Context, fromDID, nonce string, expiresAt time.Time) (bool, error) {
	if _, err := n.DB.ExecContext(ctx, `DELETE FROM peer_challenge_nonces WHERE expires_at < now()`); err != nil {
		return false, err
	}
	result, err := n.DB.ExecContext(ctx, `
		INSERT INTO peer_challenge_nonces (from_did, nonce, expires_at) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, fromDID, nonce, expiresAt)
	if err != nil {
		return false, err
	}
	inserted, err := result.RowsAffected()
	return inserted == 1, err
}

// PeerRequest is what an inbound peer request claims and carries.
type PeerRequest struct {
	Op          PeerOp
	FromDID     string
	ContractIRI string
	Body        []byte
	SecretValue string
	SecretHash  []byte
}

// VerifyPeerRequest authenticates one inbound peer request. The challenge must
// name this operation, this contract, the requesting peer, this instance and
// the body, its iat must be within PeerChallengeWindow of now, the signature
// must verify against the peer's DID document, and its nonce must be new. The
// nonce is recorded only after the signature verified, so a caller without
// the peer's key cannot fill the nonce table.
func VerifyPeerRequest(ctx context.Context, nonces PeerNonces, peerDocument *identity.DIDDocument, trustPool *identity.EUTrustPool, localPeer string, req PeerRequest, now time.Time) error {
	fields := strings.Split(req.SecretValue, "|")
	if len(fields) != 9 || fields[2] != peerChallengeVersion {
		return fmt.Errorf("%w: the secret is not a %s challenge", ErrPeerChallengeRefused, peerChallengeVersion)
	}
	op, contractIRI, route, fromDID, toDID, issuedAt, nonce, digest :=
		fields[0], fields[1], fields[3], fields[4], fields[5], fields[6], fields[7], fields[8]
	switch {
	case op != req.Op.Name || route != req.Op.Route:
		return fmt.Errorf("%w: the challenge is signed for %s (%s), not for %s", ErrPeerChallengeRefused, op, route, req.Op.Name)
	case contractIRI != req.ContractIRI:
		return fmt.Errorf("%w: the challenge names contract %s, not %s", ErrPeerChallengeRefused, contractIRI, req.ContractIRI)
	case fromDID != req.FromDID:
		return fmt.Errorf("%w: the challenge is signed by %s, not by %s", ErrPeerChallengeRefused, fromDID, req.FromDID)
	case !identity.SameDIDWeb(toDID, localPeer):
		return fmt.Errorf("%w: the challenge is addressed to %s, not to this instance %s", ErrPeerChallengeRefused, toDID, localPeer)
	case digest != bodyDigest(req.Body):
		return fmt.Errorf("%w: the challenge does not bind the body of this request", ErrPeerChallengeRefused)
	case nonce == "":
		return fmt.Errorf("%w: the challenge carries no nonce", ErrPeerChallengeRefused)
	}
	seconds, err := strconv.ParseInt(issuedAt, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: iat %q is not a unix time", ErrPeerChallengeRefused, issuedAt)
	}
	iat := time.Unix(seconds, 0)
	if skew := now.Sub(iat); skew > PeerChallengeWindow || skew < -PeerChallengeWindow {
		return fmt.Errorf("%w: iat is %s away from this instance's clock, more than %s (check NTP on both instances)",
			ErrPeerChallengeRefused, skew.Round(time.Second), PeerChallengeWindow)
	}
	if err := peerDocument.VerifyPeerChallenge(trustPool, []byte(req.SecretValue), req.SecretHash); err != nil {
		return fmt.Errorf("%w: %w", ErrPeerChallengeRefused, err)
	}
	// Kept for twice the window, so a clock difference between this instance
	// and the database cannot drop a nonce while its challenge is still valid.
	fresh, err := nonces.Use(ctx, fromDID, nonce, iat.Add(2*PeerChallengeWindow))
	if err != nil {
		return fmt.Errorf("record the peer challenge nonce: %w", err)
	}
	if !fresh {
		return fmt.Errorf("%w: the challenge was already used", ErrPeerChallengeRefused)
	}
	return nil
}
