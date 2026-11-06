package service

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
	"time"

	goa "goa.design/goa/v3/pkg"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
)

func TestContractInfoForPeerAnswersOnlyAParty(t *testing.T) {
	const peer = "did:web:peer.example"
	data := datatype.JSON(`{"@id":"x"}`)
	contract := &db.Contract{
		DID: eraseTestIRI, State: "SIGNED", ContractVersion: 3, UpdatedAt: time.Unix(0, 0),
		ContractData: &data,
		Responsible:  &db.Responsible{Creator: "did:web:local.example", Counterparty: peer},
	}

	info, err := contractInfoForPeer(contract, peer, eraseTestIRI)
	if err != nil {
		t.Fatalf("a party must get the contract information: %v", err)
	}
	if info.State != "SIGNED" || info.ContractVersion != 3 || info.ContractDocumentDigest == "" {
		t.Fatalf("unexpected information %+v", info)
	}

	for name, c := range map[string]*db.Contract{
		"missing":   nil,
		"no record": {DID: eraseTestIRI},
		"other parties": {DID: eraseTestIRI, Responsible: &db.Responsible{
			Creator: "did:web:local.example", Counterparty: "did:web:someone-else.example"}},
	} {
		_, err := contractInfoForPeer(c, peer, eraseTestIRI)
		var serviceErr *goa.ServiceError
		if !errors.As(err, &serviceErr) || serviceErr.Name != "not_found" {
			t.Errorf("%s: want not_found, got %v", name, err)
		}
	}
}

func TestGetContractInfoRejectsFailedChallenge(t *testing.T) {
	peerDoc := eraseTestDIDDocument(t, "peer.example")
	svc, _, restore := newEraseTestService(t, peerDoc, nil)
	defer restore()

	wrongDoc := eraseTestDIDDocument(t, "peer.example")
	hash, err := wrongDoc.Sign([]byte("challenge"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetContractInfo(context.Background(), &dcstodcs.DCSToDCSContractInfoRequest{
		FromPeerDid: "did:web:peer.example", ContractIri: eraseTestIRI, SecretValue: "challenge", SecretHash: hash,
	}); err == nil {
		t.Fatal("a failed challenge must reject the request")
	}
}

// The peer authentication and the federation policy gate run before the
// contract is read.
func TestGetContractInfoReadsNothingBeforeTheGates(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "dcs_to_dcs_info.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body *ast.BlockStmt
	for _, decl := range parsed.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "GetContractInfo" {
			body = fn.Body
		}
	}
	if body == nil {
		t.Fatal("dcs_to_dcs_info.go declares no GetContractInfo")
	}
	read := callSites(body, "ExistsByDID")
	if len(read) == 0 {
		t.Fatal("GetContractInfo does not read the contract")
	}
	for _, gate := range []string{"admitPeer", "contractInfoForPeer"} {
		sites := callSites(body, gate)
		if len(sites) == 0 {
			t.Errorf("GetContractInfo does not call %s", gate)
		} else if gate != "contractInfoForPeer" && sites[0] > read[0] {
			t.Errorf("%s runs after the contract is read", gate)
		}
	}
}
