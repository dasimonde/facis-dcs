package service

import (
	"context"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/middleware"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	participantcmd "digital-contracting-service/internal/templatecatalogueintegration/command/participant"
	serviceofferingcmd "digital-contracting-service/internal/templatecatalogueintegration/command/serviceoffering"
	participantquery "digital-contracting-service/internal/templatecatalogueintegration/query/participant"
	serviceofferingquery "digital-contracting-service/internal/templatecatalogueintegration/query/serviceoffering"
	templatequery "digital-contracting-service/internal/templatecatalogueintegration/query/template"
	selfdescription "digital-contracting-service/internal/templatecatalogueintegration/selfdescription"
	"errors"
	"fmt"
)

type templateCatalogueIntegrationsrvc struct {
	auth.JWTAuthenticator
	fcClient *fcclient.FederatedCatalogueClient
}

func NewTemplateCatalogueIntegration(jwtAuth auth.JWTAuthenticator, fcClient *fcclient.FederatedCatalogueClient) templatecatalogueintegration.Service {
	return &templateCatalogueIntegrationsrvc{JWTAuthenticator: jwtAuth, fcClient: fcClient}
}

func (s *templateCatalogueIntegrationsrvc) RetrieveTemplate(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueRetrieveRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveResponse, err error) {
	queryHandler := templatequery.GetAllMetadataHandler{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := queryHandler.Handle(templatequery.GetAllMetadataQry{
		Offset: req.Offset,
		Limit:  req.Limit,
	})
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}
	return result, nil
}

func (s *templateCatalogueIntegrationsrvc) RetrieveTemplateByID(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueRetrieveByIDRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveByIDResponse, err error) {
	queryHandler := templatequery.GetByIDHandler{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := queryHandler.Handle(templatequery.GetByIDQry{
		DID:     req.Did,
		Version: req.Version,
	})
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}
	if result == nil {
		return nil, nil
	}

	return result, nil
}

// Create a new participant in the Federated Catalogue.
// A new participant group will be created in the Keycloak.
func (s *templateCatalogueIntegrationsrvc) CreateParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueCreateParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueCreateParticipantResponse, err error) {

	createHandler := participantcmd.Creator{
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

	cmd := participantcmd.CreateCmd{
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
	}
	err = createHandler.Handle(ctx, cmd)
	if err != nil {
		if errors.Is(err, participantcmd.ErrParticipantAlreadyExists) {
			return nil, templatecatalogueintegration.MakeBadRequest(err)
		}
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	participantID := middleware.GetParticipantID(ctx)
	return &templatecatalogueintegration.TemplateCatalogueCreateParticipantResponse{
		ID: participantID,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) CreateServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingResponse, err error) {
	createHandler := serviceofferingcmd.Creator{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	cmd := serviceofferingcmd.CreateCmd{
		ParticipantID:      middleware.GetParticipantID(ctx),
		Description:        req.Description,
		Keywords:           req.Keywords,
		EndPointURL:        req.EndPointURL,
		TermsAndConditions: req.TermsAndConditions,
	}
	result, err := createHandler.Handle(ctx, cmd)
	if err != nil {
		if errors.Is(err, serviceofferingcmd.ErrServiceOfferingAlreadyExists) {
			return nil, templatecatalogueintegration.MakeBadRequest(err)
		}
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	return &templatecatalogueintegration.TemplateCatalogueCreateServiceOfferingResponse{
		ID: result.ID,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) GetCurrentParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantResponse, err error) {
	queryHandler := participantquery.GetCurrentParticipantHandler{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := queryHandler.Handle(participantquery.GetCurrentParticipantQry{
		ParticipantID: middleware.GetParticipantID(ctx),
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
			Country:       &result.HeadquarterAddress.Country,
			StreetAddress: &result.HeadquarterAddress.StreetAddress,
			PostalCode:    &result.HeadquarterAddress.PostalCode,
			Locality:      &result.HeadquarterAddress.Locality,
		},
		LegalAddress: &templatecatalogueintegration.TemplateCatalogueAddress{
			Country:       &result.LegalAddress.Country,
			StreetAddress: &result.LegalAddress.StreetAddress,
			PostalCode:    &result.LegalAddress.PostalCode,
			Locality:      &result.LegalAddress.Locality,
		},
		TermsAndConditions: result.TermsAndConditions,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) GetCurrentParticipantSummary(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueGetCurrentParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueParticipantSummary, err error) {
	queryHandler := participantquery.GetCurrentParticipantSummaryHandler{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := queryHandler.Handle(participantquery.GetCurrentParticipantSummaryQry{
		ParticipantID: middleware.GetParticipantID(ctx),
	})
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}
	if result == nil {
		return nil, templatecatalogueintegration.MakeNotFound(fmt.Errorf("participant not found"))
	}

	return result, nil
}

func (s *templateCatalogueIntegrationsrvc) ListOtherParticipants(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueListOtherParticipantsRequest) (res []*templatecatalogueintegration.TemplateCatalogueParticipantSummary, err error) {
	queryHandler := participantquery.GetOtherParticipantsHandler{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := queryHandler.Handle(participantquery.GetOtherParticipantsQry{
		ParticipantID: middleware.GetParticipantID(ctx),
	})
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	return result, nil
}

func (s *templateCatalogueIntegrationsrvc) GetCurrentServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueGetCurrentServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueGetCurrentServiceOfferingResponse, err error) {
	queryHandler := serviceofferingquery.GetByParticipantHandler{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	result, err := queryHandler.Handle(serviceofferingquery.GetByParticipantQry{
		ParticipantID: middleware.GetParticipantID(ctx),
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
	updateHandler := participantcmd.Updater{
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

	cmd := participantcmd.UpdateCmd{
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
	}
	err = updateHandler.Handle(ctx, cmd)
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	participantID := middleware.GetParticipantID(ctx)
	return &templatecatalogueintegration.TemplateCatalogueUpdateParticipantResponse{
		ID: participantID,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) UpdateServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueUpdateServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueUpdateServiceOfferingResponse, err error) {
	updateHandler := serviceofferingcmd.Updater{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	cmd := serviceofferingcmd.UpdateCmd{
		ParticipantID:      middleware.GetParticipantID(ctx),
		Keywords:           req.Keywords,
		Description:        req.Description,
		EndPointURL:        req.EndPointURL,
		TermsAndConditions: req.TermsAndConditions,
	}
	result, err := updateHandler.Handle(ctx, cmd)
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	return &templatecatalogueintegration.TemplateCatalogueUpdateServiceOfferingResponse{
		ID: result.ID,
	}, nil
}

// Delete the current participant from the Federated Catalogue.
// The participant group will be deleted from the Keycloak.
func (s *templateCatalogueIntegrationsrvc) DeleteParticipant(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueDeleteParticipantRequest) (res *templatecatalogueintegration.TemplateCatalogueDeleteParticipantResponse, err error) {
	deleteHandler := participantcmd.Deleter{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	cmd := participantcmd.DeleteCmd{
		ID: middleware.GetParticipantID(ctx),
	}
	err = deleteHandler.Handle(ctx, cmd)
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	participantID := middleware.GetParticipantID(ctx)
	return &templatecatalogueintegration.TemplateCatalogueDeleteParticipantResponse{
		ID: participantID,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) DeleteServiceOffering(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueDeleteServiceOfferingRequest) (res *templatecatalogueintegration.TemplateCatalogueDeleteServiceOfferingResponse, err error) {
	deleteHandler := serviceofferingcmd.Deleter{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	cmd := serviceofferingcmd.DeleteCmd{
		ParticipantID: middleware.GetParticipantID(ctx),
	}
	result, err := deleteHandler.Handle(ctx, cmd)
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}

	return &templatecatalogueintegration.TemplateCatalogueDeleteServiceOfferingResponse{
		ID: result.ID,
	}, nil
}

func (s *templateCatalogueIntegrationsrvc) SearchTemplate(ctx context.Context, req *templatecatalogueintegration.TemplateCatalogueSearchRequest) (res *templatecatalogueintegration.TemplateCatalogueRetrieveResponse, err error) {
	queryHandler := templatequery.SearchHandler{
		Ctx:      ctx,
		FCClient: s.fcClient,
	}

	qry := templatequery.SearchQry{
		DID:            derefString(req.Did),
		DocumentNumber: derefString(req.DocumentNumber),
		Version:        derefInt(req.Version),
		Name:           derefString(req.Name),
		Description:    derefString(req.Description),
		Offset:         req.Offset,
		Limit:          req.Limit,
	}

	result, err := queryHandler.Handle(qry)
	if err != nil {
		return nil, templatecatalogueintegration.MakeInternalError(err)
	}
	return result, nil
}

// derefString safely dereferences a *string.
// It returns an empty string when the pointer is nil.
func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
