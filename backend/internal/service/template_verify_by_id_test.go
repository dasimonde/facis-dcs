package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	goahttp "goa.design/goa/v3/http"
	"goa.design/goa/v3/security"

	tplserver "digital-contracting-service/gen/http/template_repository/server"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/semantichub"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
)

type verifyByIDRecorder struct {
	templaterepository.Service
	did string
}

func (r *verifyByIDRecorder) JWTAuth(ctx context.Context, _ string, _ *security.JWTScheme) (context.Context, error) {
	return ctx, nil
}

func (r *verifyByIDRecorder) VerifyByID(_ context.Context, req *templaterepository.ContractTemplateVerifyRequest) (*templaterepository.ContractTemplateVerifyResponse, error) {
	r.did = req.Did
	return &templaterepository.ContractTemplateVerifyResponse{Did: req.Did, Findings: []string{}}, nil
}

// DCS-FR-TR-20: GET /template/verify/{template_id} verifies the template the
// path names, under the scopes of POST /template/verify.
func TestTemplateVerifyByIDRoute(t *testing.T) {
	ctx := context.Background()
	verify := requiredTemplateScopes(t, func(e *templaterepository.Endpoints) error {
		_, err := e.Verify(ctx, &templaterepository.ContractTemplateVerifyRequest{})
		return err
	})
	byID := requiredTemplateScopes(t, func(e *templaterepository.Endpoints) error {
		_, err := e.VerifyByID(ctx, &templaterepository.ContractTemplateVerifyRequest{})
		return err
	})
	require.Equal(t, verify, byID)

	recorder := &verifyByIDRecorder{}
	mux := goahttp.NewMuxer()
	tplserver.Mount(mux, tplserver.New(templaterepository.NewEndpoints(recorder), mux, goahttp.RequestDecoder, goahttp.ResponseEncoder, nil, nil))
	request := httptest.NewRequest(http.MethodGet, "/template/verify/did:web:example.com:template:1", nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, "did:web:example.com:template:1", recorder.did)
}

// DCS-FR-TR-20: the path route runs the checks of POST /template/verify.
func TestTemplateVerifyByIDRunsTheVerifyChecksPostgres(t *testing.T) {
	database := pgtest.Open(t)
	require.NoError(t, semantichub.Seed(context.Background(), database))
	validation.SetShapeSource(semantichub.HubShapeSource{DB: database})
	_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name,template_data,version,base_template) VALUES ('tpl-unsealed','creator','REGISTERED','CONTRACT_TEMPLATE','SINGLE_PARTY','n',$1,1,'tpl-unsealed')`, compliantTemplateData)
	require.NoError(t, err)
	svc := &templateRepositorysrvc{DB: database, CTRepo: &pg.PostgresContractTemplateRepo{}, RTRepo: &pg.PostgresReviewTaskRepo{}}
	ctx := middleware.InjectAuthContext(context.Background(), []string{userrole.ContractCreator.String()}, "holder", "org")
	req := &templaterepository.ContractTemplateVerifyRequest{Did: "tpl-unsealed"}

	byBody, err := svc.Verify(ctx, req)
	require.NoError(t, err)
	byPath, err := svc.VerifyByID(ctx, req)
	require.NoError(t, err)

	require.NotEmpty(t, byPath.Findings)
	require.Equal(t, byBody, byPath)
}
