package service

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/middleware"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templatecatalogueintegration/command"
	selfdescription "digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
	"errors"
	"fmt"

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
	legalCountry := ""
	legalStreet := ""
	legalPostal := ""
	legalLocality := ""
	if req.HeadquarterAddress != nil {
		headquarterCountry = derefString(req.HeadquarterAddress.Country)
		headquarterStreet = derefString(req.HeadquarterAddress.StreetAddress)
		headquarterPostal = derefString(req.HeadquarterAddress.PostalCode)
		headquarterLocality = derefString(req.HeadquarterAddress.Locality)
	}
	if req.LegalAddress != nil {
		legalCountry = derefString(req.LegalAddress.Country)
		legalStreet = derefString(req.LegalAddress.StreetAddress)
		legalPostal = derefString(req.LegalAddress.PostalCode)
		legalLocality = derefString(req.LegalAddress.Locality)
	}

	result, err := handler.Handle(command.CreateParticipantCmd{
		Token: *req.Token,
		Participant: selfdescription.ParticipantSdInput{
			ParticipantID:             middleware.GetParticipantID(ctx),
			LegalName:                 req.LegalName,
			RegistrationNumber:        req.RegistrationNumber,
			LeiCode:                   req.LeiCode,
			EthereumAddress:           req.EthereumAddress,
			HeadquarterCountry:        headquarterCountry,
			HeadquarterStreetAddress:  headquarterStreet,
			HeadquarterPostalCode:     headquarterPostal,
			HeadquarterLocality:       headquarterLocality,
			LegalAddressCountry:       legalCountry,
			LegalAddressStreetAddress: legalStreet,
			LegalAddressPostalCode:    legalPostal,
			LegalAddressLocality:      legalLocality,
			TermsAndConditions:        req.TermsAndConditions,
		},
	})
	if err != nil {
		if errors.Is(err, command.ErrParticipantAlreadyExists) {
			return nil, templatecatalogueintegration.MakeBadRequest(err)
		}
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	return &templatecatalogueintegration.TemplateCatalogueCreateParticipantResponse{
		ID: result.ID,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) CreateServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingResponse, err error) {
	handler := command.CreateServiceOffering{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := handler.Handle(command.CreateServiceOfferingCmd{
		Token:              *req.Token,
		ParticipantID:      middleware.GetParticipantID(ctx),
		Description:        req.Description,
		Keywords:           req.Keywords,
		EndPointURL:        req.EndPointURL,
		TermsAndConditions: req.TermsAndConditions,
	})
	if err != nil {
		if errors.Is(err, command.ErrServiceOfferingAlreadyExists) {
			return nil, templatecatalogueintegration.MakeBadRequest(err)
		}
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	return &templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingResponse{
		ID: result.ID,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) GetCurrentParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantResponse, err error) {
	handler := command.GetCurrentParticipant{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := handler.Handle(command.GetCurrentParticipantCmd{
		ParticipantID: middleware.GetParticipantID(ctx),
		Token:         *req.Token,
	})
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}
	if result == nil {
		return nil, templatecatalogueintegration.MakeNotFound(fmt.Errorf("participant not found"))
	}

	return &templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantResponse{
		LegalName:          result.LegalName,
		RegistrationNumber: result.RegistrationNumber,
		LeiCode:            result.LeiCode,
		EthereumAddress:    result.EthereumAddress,
		HeadquarterAddress: &templatecatalogueintegration.TemplateCatalogueHeadquarterAddress{
			Country:       &result.HeadquarterCountry,
			StreetAddress: &result.HeadquarterStreet,
			PostalCode:    &result.HeadquarterPostal,
			Locality:      &result.HeadquarterLocality,
		},
		LegalAddress: &templatecatalogueintegration.TemplateCatalogueAddress{
			Country:       &result.LegalCountry,
			StreetAddress: &result.LegalStreet,
			PostalCode:    &result.LegalPostal,
			Locality:      &result.LegalLocality,
		},
		TermsAndConditions: result.TermsAndConditions,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) GetCurrentServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueGetCurrentServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueGetCurrentServiceOfferingResponse, err error) {
	handler := command.GetCurrentServiceOffering{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := handler.Handle(command.GetCurrentServiceOfferingCmd{
		ParticipantID: middleware.GetParticipantID(ctx),
		Token:         *req.Token,
	})
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}
	if result == nil {
		return nil, templatecatalogueintegration.MakeNotFound(fmt.Errorf("service offering not found"))
	}

	return &templatecatalogueintegration.TemplateCatalogueGetCurrentServiceOfferingResponse{
		Keywords:           result.Keywords,
		Description:        result.Description,
		EndPointURL:        result.EndPointURL,
		TermsAndConditions: result.TermsAndConditions,
	}, nil
}
func (s *templateCatalogueIntegrationsrvc) UpdateParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueUpdateParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueUpdateParticipantResponse, err error) {
	handler := command.UpdateParticipant{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	headquarterCountry := ""
	headquarterStreet := ""
	headquarterPostal := ""
	headquarterLocality := ""
	legalCountry := ""
	legalStreet := ""
	legalPostal := ""
	legalLocality := ""
	if req.HeadquarterAddress != nil {
		headquarterCountry = derefString(req.HeadquarterAddress.Country)
		headquarterStreet = derefString(req.HeadquarterAddress.StreetAddress)
		headquarterPostal = derefString(req.HeadquarterAddress.PostalCode)
		headquarterLocality = derefString(req.HeadquarterAddress.Locality)
	}
	if req.LegalAddress != nil {
		legalCountry = derefString(req.LegalAddress.Country)
		legalStreet = derefString(req.LegalAddress.StreetAddress)
		legalPostal = derefString(req.LegalAddress.PostalCode)
		legalLocality = derefString(req.LegalAddress.Locality)
	}

	result, err := handler.Handle(command.UpdateParticipantCmd{
		Token: *req.Token,
		Participant: selfdescription.ParticipantSdInput{
			ParticipantID:             middleware.GetParticipantID(ctx),
			LegalName:                 req.LegalName,
			RegistrationNumber:        req.RegistrationNumber,
			LeiCode:                   req.LeiCode,
			EthereumAddress:           req.EthereumAddress,
			HeadquarterCountry:        headquarterCountry,
			HeadquarterStreetAddress:  headquarterStreet,
			HeadquarterPostalCode:     headquarterPostal,
			HeadquarterLocality:       headquarterLocality,
			LegalAddressCountry:       legalCountry,
			LegalAddressStreetAddress: legalStreet,
			LegalAddressPostalCode:    legalPostal,
			LegalAddressLocality:      legalLocality,
			TermsAndConditions:        req.TermsAndConditions,
		},
	})
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}
	if result == nil {
		return nil, templatecatalogueintegration.MakeNotFound(fmt.Errorf("participant not found"))
	}

	return &templatecatalogueintegration.TemplateCatalogueUpdateParticipantResponse{
		ID: result.ID,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) UpdateServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueUpdateServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueUpdateServiceOfferingResponse, err error) {
	endPointURL := req.EndPointURL
	log.Printf(ctx, "templateCatalogueIntegration.updateServiceOffering endPointURL=%s", endPointURL)
	return &templatecatalogueintegration.TemplateCatalogueUpdateServiceOfferingResponse{
		ID: "",
	}, nil
}

// Delete the current participant from the Federated Catalogue.
// The participant group will be deleted from the Keycloak.
func (s *templateCatalogueIntegrationsrvc) DeleteParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueDeleteParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueDeleteParticipantResponse, err error) {
	handler := command.DeleteParticipant{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := handler.Handle(command.DeleteParticipantCmd{
		ID:    middleware.GetParticipantID(ctx),
		Token: *req.Token,
	})
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}
	if result == nil {
		return &templatecatalogueintegration.TemplateCatalogueDeleteParticipantResponse{
			ID: middleware.GetParticipantID(ctx),
		}, nil
	}

	return &templatecatalogueintegration.TemplateCatalogueDeleteParticipantResponse{
		ID: result.ID,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) DeleteServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueDeleteServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueDeleteServiceOfferingResponse, err error) {
	log.Printf(ctx, "templateCatalogueIntegration.deleteServiceOffering")
	return &templatecatalogueintegration.TemplateCatalogueDeleteServiceOfferingResponse{
		ID: "",
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
