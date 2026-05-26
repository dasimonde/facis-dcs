package contracttemplate

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
)

type GetTemplateDataByDIDQry struct {
	DID string
}

type GetTemplateDataByDIDHandler struct {
	Ctx      context.Context
	DB       *sqlx.DB
	CTRepo   db.ContractTemplateRepo
	FCClient *fcclient.FederatedCatalogueClient
}

var templateDataAllowedKeys = []string{
	"documentOutline",
	"documentBlocks",
	"semanticConditions",
	"subTemplateSnapshots",
	"templateDataVersion",
}

const getTemplateDataJSONByDIDStatement = `
MATCH (ct:ContractTemplate)
WHERE ct.did = $did
RETURN {
  template_data_json: ct.templateDataJSON
} AS n
LIMIT 1
`

func (h *GetTemplateDataByDIDHandler) Handle(ctx context.Context, qry GetTemplateDataByDIDQry) (*datatype.JSON, error) {
	templateData, err := h.getTemplateData(ctx, qry)
	if err != nil {
		return nil, err
	}
	return convertTemplateDataToContractData(templateData)
}

func (h *GetTemplateDataByDIDHandler) getTemplateData(ctx context.Context, qry GetTemplateDataByDIDQry) (*datatype.JSON, error) {

	templateData, err := h.getFrameContractTemplateDataFromDB(ctx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read template data from DB: %w", err)
	}

	if templateData == nil && h.FCClient != nil {
		templateData, err = h.getTemplateDataFromFC(qry)
		if err != nil {
			return nil, fmt.Errorf("could not read template data from FC: %w", err)
		}
	}

	return templateData, nil
}

func (h *GetTemplateDataByDIDHandler) getFrameContractTemplateDataFromDB(ctx context.Context, templateDID string) (*datatype.JSON, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer tx.Rollback()

	templateData, err := h.CTRepo.ReadFrameContractTemplateDataByID(ctx, tx, templateDID)
	if err != nil {
		return nil, fmt.Errorf("could not read frame contract template data: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}
	return templateData, nil
}

func (h *GetTemplateDataByDIDHandler) getTemplateDataFromFC(qry GetTemplateDataByDIDQry) (*datatype.JSON, error) {
	resp, err := h.FCClient.Query(h.Ctx, fcclient.QueryRequest{
		Statement: getTemplateDataJSONByDIDStatement,
		Parameters: map[string]string{
			"did": qry.DID,
		},
	})
	if err != nil {
		return nil, err
	}
	if resp.TotalCount == 0 || len(resp.Items) == 0 {
		return nil, fmt.Errorf("template data not found for did %s", qry.DID)
	}

	var projection map[string]interface{}
	for _, v := range resp.Items[0] {
		if m, ok := v.(map[string]interface{}); ok {
			projection = m
			break
		}
	}
	if projection == nil {
		return nil, fmt.Errorf("query projection missing projected map for did=%s", qry.DID)
	}

	templateDataJSONString, _ := projection["template_data_json"].(string)
	if strings.TrimSpace(templateDataJSONString) == "" {
		return nil, fmt.Errorf("templateDataJSON is empty for did %s", qry.DID)
	}

	var templateDataMap map[string]interface{}
	if err := json.Unmarshal([]byte(templateDataJSONString), &templateDataMap); err != nil {
		return nil, fmt.Errorf("unmarshal templateDataJSON failed: %w", err)
	}

	templateData, err := datatype.NewJSON(templateDataMap)
	if err != nil {
		return nil, fmt.Errorf("marshal template data failed: %w", err)
	}

	return &templateData, nil
}

func convertTemplateDataToContractData(raw *datatype.JSON) (*datatype.JSON, error) {
	if raw == nil || !raw.IsNotNullValue() {
		return raw, nil
	}

	var templateDataMap map[string]interface{}
	if err := json.Unmarshal(*raw, &templateDataMap); err != nil {
		return nil, fmt.Errorf("unmarshal template data failed: %w", err)
	}

	contractDataMap := make(map[string]interface{}, len(templateDataAllowedKeys))
	for _, key := range templateDataAllowedKeys {
		if value, ok := templateDataMap[key]; ok {
			contractDataMap[key] = value
		}
	}

	contractData, err := datatype.NewJSON(contractDataMap)
	if err != nil {
		return nil, fmt.Errorf("marshal converted contract data failed: %w", err)
	}
	return &contractData, nil
}
