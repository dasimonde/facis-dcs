package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"digital-contracting-service/internal/base/jades"
	"digital-contracting-service/internal/contractworkflowengine/db"
	trustgate "digital-contracting-service/internal/dcstodcs"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
)

// GetContractInfo returns read-only information about a contract to an
// authenticated counterparty DCS (DCS-IR-SI-06). The peer passes the same
// challenge-response and federation trust gate as post_pdf, and must be a
// party of the contract.
func (s *dcsToDcssrvc) GetContractInfo(ctx context.Context, req *dcstodcs.DCSToDCSContractInfoRequest) (res *dcstodcs.DCSToDCSContractInfoResponse, err error) {
	defer func() { s.auditReceivedExchange(ctx, "get_contract_info", req.ContractIri, req.FromPeerDid, err) }()
	if _, _, err := s.admitPeer(ctx, trustgate.OpGetContractInfo, req.FromPeerDid, req.ContractIri, nil, req.SecretValue, req.SecretHash); err != nil {
		return nil, err
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()
	exists, err := s.CRepo.ExistsByDID(ctx, tx, req.ContractIri)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	var contract *db.Contract
	if exists {
		if contract, err = s.CRepo.ReadDataByDID(ctx, tx, req.ContractIri); err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
	}
	return contractInfoForPeer(contract, req.FromPeerDid, req.ContractIri)
}

// contractInfoForPeer answers only a peer that is a party of the contract. A
// missing contract and a contract of other parties give the same answer, so
// the endpoint does not tell a peer which contracts exist.
func contractInfoForPeer(contract *db.Contract, peerDID, contractIRI string) (*dcstodcs.DCSToDCSContractInfoResponse, error) {
	if contract == nil || contract.Responsible == nil || !slices.Contains(contract.Responsible.GetParties(), peerDID) {
		return nil, dcstodcs.MakeNotFound(fmt.Errorf("no contract %s is shared with peer %s", contractIRI, peerDID))
	}
	digest, err := jades.ContractDocumentDigest(contractDocumentOf(contract))
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}
	return &dcstodcs.DCSToDCSContractInfoResponse{
		Did:                    contract.DID,
		State:                  contract.State,
		ContractVersion:        contract.ContractVersion,
		ContractDocumentDigest: digest,
		UpdatedAt:              contract.UpdatedAt.UTC().Format(time.RFC3339),
	}, nil
}
