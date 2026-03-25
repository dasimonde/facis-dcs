package service

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/middleware"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/command"

	"goa.design/clue/log"
)

type templateCatalogueIntegrationsrvc struct {
	auth.JWTAuthenticator
	fcClient *fcclient.FederatedCatalogueClient
}

func NewTemplateCatalogueIntegration(jwtAuth auth.JWTAuthenticator, fcClient *fcclient.FederatedCatalogueClient) templatecatalogueintegration.Service {
	return &templateCatalogueIntegrationsrvc{JWTAuthenticator: jwtAuth, fcClient: fcClient}
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

// Create a new participant in the Federated Catalogue.
// A new participant group will be created in the Keycloak.
func (s *templateCatalogueIntegrationsrvc) CreateParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueCreateParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueCreateParticipantResponse, err error) {

	handler := command.CreateParticipant{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	headquarterCountry := ""
	headquarterStreet := ""
	headquarterPostal := ""
	headquarterLocality := ""
	legalStreet := ""
	legalPostal := ""
	legalLocality := ""
	if req.HeadquarterAddress != nil {
		headquarterCountry = derefString(req.HeadquarterAddress.Country)
		headquarterStreet = derefString(req.HeadquarterAddress.StreetAddress)
		headquarterPostal = derefString(req.HeadquarterAddress.PostalCode)
		headquarterLocality = derefString(req.HeadquarterAddress.Locality)
		if req.HeadquarterAddress.LegalAddress != nil {
			legalStreet = derefString(req.HeadquarterAddress.LegalAddress.StreetAddress)
			legalPostal = derefString(req.HeadquarterAddress.LegalAddress.PostalCode)
			legalLocality = derefString(req.HeadquarterAddress.LegalAddress.Locality)
		}
	}

	result, err := handler.Handle(command.CreateParticipantCmd{
		Token:               *req.Token,
		ParticipantID:       middleware.GetParticipantID(ctx),
		LegalName:           req.LegalName,
		RegistrationNumber:  req.RegistrationNumber,
		LeiCode:             req.LeiCode,
		EthereumAddress:     req.EthereumAddress,
		HeadquarterCountry:  headquarterCountry,
		HeadquarterStreet:   headquarterStreet,
		HeadquarterPostal:   headquarterPostal,
		HeadquarterLocality: headquarterLocality,
		LegalStreet:         legalStreet,
		LegalPostal:         legalPostal,
		LegalLocality:       legalLocality,
		TermsAndConditions:  req.TermsAndConditions,
	})
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	return &templatecatalogueintegration.TemplateCatalogueCreateParticipantResponse{
		ID: result.ID,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) CreateServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingResponse, err error) {
	log.Printf(ctx, "templateCatalogueIntegration.createServiceOffering")
	return &templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingResponse{
		SdHash: "",
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) GetCurrentParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantResponse, err error) {
	participantID := middleware.GetParticipantID(ctx)
	if participantID == "" {
		log.Printf(ctx, "templateCatalogueIntegration.getCurrentParticipant participant_id_missing")
		return nil, nil
	}
	log.Printf(ctx, "templateCatalogueIntegration.getCurrentParticipant participant_id=%s", participantID)
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

// derefString safely dereferences a *string.
// It returns an empty string when the pointer is nil.
func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
