package service

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/auth"

	"goa.design/clue/log"
)

type templateCatalogueIntegrationsrvc struct {
	auth.JWTAuthenticator
}

func NewTemplateCatalogueIntegration(jwtAuth auth.JWTAuthenticator) templatecatalogueintegration.Service {
	return &templateCatalogueIntegrationsrvc{JWTAuthenticator: jwtAuth}
}

func (s *templateCatalogueIntegrationsrvc) Retrieve(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueRetrieveRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveResponse, err error) {
	log.Printf(ctx, "templateCatalogueIntegration.retrieve")
	return &templatecatalogueintegration.TemplateCatalogueRetrieveResponse{}, nil
}

func (s *templateCatalogueIntegrationsrvc) RetrieveByID(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueRetrieveByIDRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse, err error) {
	did := req.Did
	log.Printf(ctx, "templateCatalogueIntegration.retrieveByID did=%s", did)
	return &templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse{
		Did: did,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) CreateParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueCreateParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueCreateParticipantResponse, err error) {
	log.Printf(ctx, "templateCatalogueIntegration.createParticipant")
	return &templatecatalogueintegration.TemplateCatalogueCreateParticipantResponse{
		SdHash: "",
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) CreateServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingResponse, err error) {
	log.Printf(ctx, "templateCatalogueIntegration.createServiceOffering")
	return &templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingResponse{
		SdHash: "",
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) GetCurrentParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantResponse, err error) {
	log.Printf(ctx, "templateCatalogueIntegration.getCurrentParticipant")
	return &templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantResponse{
		LegalName:          "",
		RegistrationNumber: "",
		LeiCode:            "",
		EthereumAddress:    "",
		HeadquarterAddress: &templatecatalogueintegration.TemplateCatalogueHeadquarterAddress{
			LegalAddress: &templatecatalogueintegration.TemplateCatalogueAddress{},
		},
		TermsAndConditions: "",
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) GetCurrentServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueGetCurrentServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueGetCurrentServiceOfferingResponse, err error) {
	log.Printf(ctx, "templateCatalogueIntegration.getCurrentServiceOffering")
	return &templatecatalogueintegration.TemplateCatalogueGetCurrentServiceOfferingResponse{
		EndPointURL: "",
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) UpdateParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueUpdateParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueUpdateParticipantResponse, err error) {
	sdHash := req.SdHash
	log.Printf(ctx, "templateCatalogueIntegration.updateParticipant sdHash=%s", sdHash)
	return &templatecatalogueintegration.TemplateCatalogueUpdateParticipantResponse{
		SdHash: sdHash,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) UpdateServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueUpdateServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueUpdateServiceOfferingResponse, err error) {
	sdHash := req.SdHash
	log.Printf(ctx, "templateCatalogueIntegration.updateServiceOffering sdHash=%s", sdHash)
	return &templatecatalogueintegration.TemplateCatalogueUpdateServiceOfferingResponse{
		SdHash: sdHash,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) DeleteParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueDeleteRequest) (res *templatecatalogueintegration.TemplateCatalogueDeleteResponse, err error) {
	sdHash := req.SdHash
	log.Printf(ctx, "templateCatalogueIntegration.deleteParticipant sdHash=%s", sdHash)
	return &templatecatalogueintegration.TemplateCatalogueDeleteResponse{
		SdHash: sdHash,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) DeleteServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueDeleteRequest) (res *templatecatalogueintegration.TemplateCatalogueDeleteResponse, err error) {
	sdHash := req.SdHash
	log.Printf(ctx, "templateCatalogueIntegration.deleteServiceOffering sdHash=%s", sdHash)
	return &templatecatalogueintegration.TemplateCatalogueDeleteResponse{
		SdHash: sdHash,
	}, nil
}
