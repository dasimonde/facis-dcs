package service

import (
	"context"

	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/middleware"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	templatequery "digital-contracting-service/internal/templatecatalogueintegration/query/template"

	"github.com/jmoiron/sqlx"
)

type templateCatalogueIntegrationsrvc struct {
	auth.JWTAuthenticator
	db       *sqlx.DB
	fcClient *fcclient.FederatedCatalogueClient
}

func NewTemplateCatalogueIntegration(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, fcClient *fcclient.FederatedCatalogueClient) templatecatalogueintegration.Service {
	return &templateCatalogueIntegrationsrvc{JWTAuthenticator: jwtAuth, db: db, fcClient: fcClient}
}

func (s *templateCatalogueIntegrationsrvc) RetrieveTemplate(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueRetrieveRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	queryHandler := templatequery.GetAllMetadataHandler{
		DB:       s.db,
		FCClient: s.fcClient,
	}

	result, err := queryHandler.Handle(ctx, templatequery.GetAllMetadataQry{
		Offset:      req.Offset,
		Limit:       req.Limit,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	})

	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	return result, nil
}

func (s *templateCatalogueIntegrationsrvc) RetrieveTemplateByID(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueRetrieveByIDRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	queryHandler := templatequery.GetByIDHandler{
		DB:       s.db,
		FCClient: s.fcClient,
	}

	result, err := queryHandler.Handle(ctx, templatequery.GetByIDQry{
		DID:         req.Did,
		Version:     req.Version,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	})

	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	if result == nil {
		return nil, nil
	}

	return result, nil
}

func (s *templateCatalogueIntegrationsrvc) SearchTemplate(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueSearchRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveResponse, err error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	queryHandler := templatequery.SearchHandler{
		DB:       s.db,
		FCClient: s.fcClient,
	}

	qry := templatequery.SearchQry{
		DID:         base.DerefString(req.Did),
		Version:     base.DerefInt(req.Version),
		Name:        base.DerefString(req.Name),
		Description: base.DerefString(req.Description),
		Offset:      req.Offset,
		Limit:       req.Limit,
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}
	return result, nil
}
