package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/negotiationmerging"
	trustgate "digital-contracting-service/internal/dcstodcs"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
)

// PostNegotiation receives a peer's Proposal, Accept or Reject. ACK means the
// domain fact is durably stored — not that the change is agreed. Accept/Reject
// whose Proposal is missing are refused so the sender retries after the
// predecessor arrives.
func (s *dcsToDcssrvc) PostNegotiation(ctx context.Context, req *dcstodcs.DCSToDCSNegotiationMessageRequest) (res *dcstodcs.DCSToDCSNegotiationMessageResponse, err error) {
	defer func() { s.auditReceivedExchange(ctx, "post_negotiation", req.ContractIri, req.FromPeerDid, err) }()
	body, err := trustgate.NegotiationMessageBody(req)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(fmt.Errorf("post_negotiation rejected: %w", err))
	}
	_, localPeer, err := s.admitPeer(ctx, trustgate.OpPostNegotiation, req.FromPeerDid, req.ContractIri, body, req.SecretValue, req.SecretHash)
	if err != nil {
		return nil, err
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	defer func() { _ = tx.Rollback() }()

	_, err = s.CRepo.ReadProcessDataByDID(ctx, tx, req.ContractIri)
	if err != nil {
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_negotiation rejected: this instance holds no copy of contract %s: %w", req.ContractIri, err))
	}

	switch req.Kind {
	case cwedb.NegotiationMessageProposal:
		err = s.persistShippedProposal(ctx, tx, localPeer, req)
		if err != nil {
			return nil, err
		}
	case cwedb.NegotiationMessageAccept, cwedb.NegotiationMessageReject:
		err = persistShippedDecision(ctx, tx, s.NRepo, req)
		if err != nil {
			return nil, err
		}
	case cwedb.NegotiationMessageWithdraw:
		err = persistShippedWithdraw(ctx, tx, s.NRepo, req)
		if err != nil {
			return nil, err
		}
	case cwedb.NegotiationMessageOfferAccepted:
		err = persistShippedOfferAccepted(ctx, tx, s.CRepo, req)
		if err != nil {
			return nil, err
		}
	default:
		return nil, contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_negotiation rejected: unknown kind %q", req.Kind))
	}

	err = tx.Commit()
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	return &dcstodcs.DCSToDCSNegotiationMessageResponse{FromPeerDid: localPeer}, nil
}

func (s *dcsToDcssrvc) persistShippedProposal(
	ctx context.Context,
	tx *sqlx.Tx,
	localPeer string,
	req *dcstodcs.DCSToDCSNegotiationMessageRequest,
) error {
	createdBy := ""
	if req.CreatedBy != nil {
		createdBy = *req.CreatedBy
	}

	createdAt := time.Now().UTC()
	if req.CreatedAt != nil && *req.CreatedAt != "" {
		parsed, err := time.Parse(time.RFC3339, *req.CreatedAt)
		if err != nil {
			return contractworkflowengine.MakeBadRequest(
				fmt.Errorf("post_negotiation rejected: created_at is not RFC3339: %w", err))
		}
		createdAt = parsed.UTC()
	}

	var changeRequest *datatype.JSON
	if req.ChangeRequest != nil {
		encoded, err := json.Marshal(req.ChangeRequest)
		if err != nil {
			return contractworkflowengine.MakeBadRequest(
				fmt.Errorf("post_negotiation rejected: change_request is not JSON: %w", err))
		}
		if _, _, err := negotiationmerging.Decode(encoded); err != nil {
			return contractworkflowengine.MakeBadRequest(fmt.Errorf("post_negotiation rejected: %w", err))
		}
		raw := datatype.JSON(encoded)
		changeRequest = &raw
	}

	data := cwedb.NegotiationData{
		ID:              req.NegotiationID,
		DID:             req.ContractIri,
		ContractVersion: req.ContractVersion,
		ChangeRequest:   changeRequest,
		CreatedBy:       createdBy,
		CreatedAt:       createdAt,
	}

	withdrawn, err := s.NRepo.IsWithdrawn(ctx, tx, req.NegotiationID)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	if withdrawn {
		// Late or re-delivered Proposal after the author withdrew — ACK so the
		// sender's outbound drains; do not resurrect the proposal.
		log.Printf(ctx, "post_negotiation: PROPOSAL %s ignored; already withdrawn", req.NegotiationID)
		return nil
	}

	err = s.NRepo.RemoteCreateOrUpdateNegotiation(ctx, tx, data)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}

	// Receiving a proposal puts a decision on this instance's table — mint the
	// local negotiation task and an open decision so Accept/Reject can resolve
	// it. EnsureOpenDecision is idempotent; do not rely on ReadDecisionID alone
	// (a missed mint left proposals invisible under the old INNER JOIN retrieve).
	_, err = s.NTRepo.Create(ctx, tx, cwedb.NegotiationTaskData{
		DID:             req.ContractIri,
		ContractVersion: req.ContractVersion,
		Negotiator:      localPeer,
		State:           negotiationtaskstate.Open.String(),
		CreatedBy:       createdBy,
	})
	if err != nil {
		return contractworkflowengine.MakeInternalError(
			fmt.Errorf("could not mint negotiation task for received proposal: %w", err))
	}

	err = s.NRepo.EnsureOpenDecision(ctx, tx, req.NegotiationID, localPeer)
	if err != nil {
		return contractworkflowengine.MakeInternalError(
			fmt.Errorf("could not open local decision for received proposal: %w", err))
	}

	// Replicate the author's instance decision slot so the receiving party can
	// see it as Not required instead of missing the peer row.
	if req.FromPeerDid != "" && !identity.SameDIDWeb(req.FromPeerDid, localPeer) {
		err = s.NRepo.EnsureOpenDecision(ctx, tx, req.NegotiationID, req.FromPeerDid)
		if err != nil {
			return contractworkflowengine.MakeInternalError(
				fmt.Errorf("could not open author decision slot for received proposal: %w", err))
		}
	}

	processData, err := s.CRepo.ReadProcessDataByDID(ctx, tx, req.ContractIri)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}

	currentState := contractstate.ContractState(processData.State)
	if currentState == contractstate.Offered {
		err = contractstate.ValidateOutcome(currentState, contractstate.EventNegotiate, contractstate.Negotiation)
		if err != nil {
			return contractworkflowengine.MakeBadRequest(err)
		}
		err = s.CRepo.UpdateState(ctx, tx, req.ContractIri, processData.State, contractstate.Negotiation.String())
		if err != nil {
			return contractworkflowengine.MakeInternalError(
				fmt.Errorf("could not move received proposal into negotiation: %w", err))
		}
	}

	return nil
}

// persistShippedOfferAccepted advances the origin's copy OFFERED → NEGOTIATION
// once the counterparty has accepted the offer. Idempotent when already past
// OFFERED. Does not mint tasks or proposals — the origin already holds those.
func persistShippedOfferAccepted(
	ctx context.Context,
	tx *sqlx.Tx,
	cRepo cwedb.ContractRepo,
	req *dcstodcs.DCSToDCSNegotiationMessageRequest,
) error {
	processData, err := cRepo.ReadProcessDataByDID(ctx, tx, req.ContractIri)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}

	currentState := contractstate.ContractState(processData.State)
	if currentState != contractstate.Offered {
		return nil
	}

	err = contractstate.ValidateOutcome(currentState, contractstate.EventNegotiate, contractstate.Negotiation)
	if err != nil {
		return contractworkflowengine.MakeBadRequest(err)
	}

	err = cRepo.UpdateState(ctx, tx, req.ContractIri, processData.State, contractstate.Negotiation.String())
	if err != nil {
		return contractworkflowengine.MakeInternalError(
			fmt.Errorf("could not move origin copy into negotiation after offer accepted: %w", err))
	}

	return nil
}

// persistShippedWithdraw deletes the proposal on this peer. Concurrent
// partition races (author WITHDRAW vs counterparty ACCEPT while the link is
// down) resolve in favour of WITHDRAW: an Accept that already landed locally
// is undone when the author's retract arrives. DeleteByID is idempotent when
// the row is already gone.
func persistShippedWithdraw(ctx context.Context, tx *sqlx.Tx, nRepo cwedb.NegotiationRepo, req *dcstodcs.DCSToDCSNegotiationMessageRequest) error {
	err := nRepo.RecordWithdrawn(ctx, tx, req.NegotiationID, req.ContractIri)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}

	err = nRepo.DeleteByID(ctx, tx, req.NegotiationID)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}
	return nil
}

func persistShippedDecision(ctx context.Context, tx *sqlx.Tx, nRepo cwedb.NegotiationRepo, req *dcstodcs.DCSToDCSNegotiationMessageRequest) error {
	existing, err := nRepo.ReadNegotiationByID(ctx, tx, req.NegotiationID)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}

	if existing == nil {
		withdrawn, err := nRepo.IsWithdrawn(ctx, tx, req.NegotiationID)
		if err != nil {
			return contractworkflowengine.MakeInternalError(err)
		}
		if withdrawn {
			// Author withdrew while this Accept/Reject was in flight. ACK so
			// the sender's outbound drains; both sides end with no proposal.
			log.Printf(ctx, "post_negotiation: %s for %s ignored; proposal already withdrawn",
				req.Kind, req.NegotiationID)
			return nil
		}
		// Proposal not held yet — refuse so the sender retries after PROPOSAL.
		return contractworkflowengine.MakeBadRequest(
			fmt.Errorf("post_negotiation rejected: proposal %s is not held yet", req.NegotiationID))
	}

	if req.DecisionID == nil || *req.DecisionID == "" {
		return contractworkflowengine.MakeBadRequest(
			errors.New("post_negotiation rejected: decision_id is required for ACCEPT/REJECT"))
	}

	if req.Negotiator == nil || *req.Negotiator == "" {
		return contractworkflowengine.MakeBadRequest(
			errors.New("post_negotiation rejected: negotiator is required for ACCEPT/REJECT"))
	}

	decision := ""
	if req.Decision != nil {
		decision = *req.Decision
	}

	if decision == "" {
		switch req.Kind {
		case cwedb.NegotiationMessageAccept:
			decision = "ACCEPTED"
		case cwedb.NegotiationMessageReject:
			decision = "REJECTED"
		}
	}
	data := cwedb.NegotiationDecisionData{
		ID:              *req.DecisionID,
		NegotiationID:   req.NegotiationID,
		Negotiator:      *req.Negotiator,
		Decision:        &decision,
		RejectionReason: req.RejectionReason,
	}

	err = nRepo.RemoteCreateOrUpdateNegotiationDecision(ctx, tx, data)
	if err != nil {
		return contractworkflowengine.MakeInternalError(err)
	}

	return nil
}
