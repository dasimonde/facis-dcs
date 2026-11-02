package service

import (
	"context"
	"testing"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
)

// UC-IF-8: the administrator cannot register or repoint a target to a
// plain-http URL on a host the deployment did not name. The refusal comes
// before anything is stored.
func TestCreateAndUpdateContractTargetRefusePlainHTTP(t *testing.T) {
	svc := &contractWorkflowEnginesrvc{}
	const url = "http://erp.example/deploy"

	_, err := svc.CreateContractTarget(context.Background(), &contractworkflowengine.ContractTargetCreateRequest{Name: "ERP", URL: url})
	requireServiceError(t, err, "bad_request")

	_, err = svc.UpdateContractTarget(context.Background(), &contractworkflowengine.ContractTargetUpdateRequest{ID: "1", Name: "ERP", URL: url})
	requireServiceError(t, err, "bad_request")
}
