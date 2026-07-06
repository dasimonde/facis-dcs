// Package testutil provides a fake/stub Federated Catalogue HTTP server for
// templaterepository's tests (register/publish flows that would otherwise
// require a real FCClient).
package testutil

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/fcasset"
	semanticmapper "digital-contracting-service/internal/semantic/mapper"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	templatedb "digital-contracting-service/internal/templaterepository/db"
	"digital-contracting-service/migrations/fcschemas"
)

//go:embed testdata/template_resource_sd.jsonld
var templateResourceSDExample []byte

const DefaultParticipantID = "did:web:argo.asd-stack.eu:facis:participant:cfc9d0a5-cd79-4807-8eef-e245ab0ffee8"

// FCClientConfig holds Federated Catalogue connection settings (read from env in test setup, not here).
type FCClientConfig struct {
	APIURL           string
	KeycloakRealmURL string
	ClientID         string
	ClientSecret     string
}

var fcSchemaSyncOnce sync.Once

// NewFCClient creates a Federated Catalogue client from the given config.
func NewFCClient(cfg FCClientConfig) (*fcclient.FederatedCatalogueClient, error) {
	return fcclient.NewFederatedCatalogueClient(fcclient.Config{
		APIURL:           cfg.APIURL,
		KeycloakRealmURL: cfg.KeycloakRealmURL,
		ClientID:         cfg.ClientID,
		ClientSecret:     cfg.ClientSecret,
	})
}

// PrepareFC cleans FC assets and syncs SHACL schemas.
func PrepareFC(t *testing.T, fc *fcclient.FederatedCatalogueClient) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	CleanupAllAssets(t, ctx, fc)
	syncFCSchemasOnce(t, ctx, fc)
}

func syncFCSchemasOnce(t *testing.T, ctx context.Context, fc *fcclient.FederatedCatalogueClient) {
	t.Helper()

	fcSchemaSyncOnce.Do(func() {
		syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err := fcschemas.SyncWithRetry(syncCtx, fc); err != nil {
			t.Fatalf("fc schema sync failed: %v", err)
		}
	})
}

// CleanupAllAssets deletes all FC assets.
func CleanupAllAssets(t *testing.T, ctx context.Context, fc *fcclient.FederatedCatalogueClient) {
	t.Helper()

	query := url.Values{}
	query.Set("statuses", "revoked,active,deprecated")
	query.Set("withMeta", "true")

	resp, err := fc.Get(ctx, fcclient.AssetsEndpointPath, query)
	if err != nil {
		t.Fatalf("list assets failed: %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := fc.ExtractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		t.Fatalf("list assets failed: %s", msg)
	}

	var listed fcclient.GetAssetsResponse
	if err := json.Unmarshal(resp.Body, &listed); err != nil {
		t.Fatalf("unmarshal assets list failed: %v", err)
	}

	for _, item := range listed.Items {
		assetHash := strings.TrimSpace(item.Meta.AssetHash)
		if assetHash == "" {
			continue
		}
		deletePath := path.Join(fcclient.AssetsEndpointPath, url.PathEscape(assetHash))
		delResp, err := fc.Delete(ctx, deletePath, nil)
		if err != nil {
			t.Fatalf("delete asset %s failed: %v", assetHash, err)
		}
		if delResp.StatusCode == http.StatusNotFound {
			continue
		}
		if delResp.StatusCode < 200 || delResp.StatusCode >= 300 {
			msg := fc.ExtractErrorMessage(delResp.Body)
			if msg == "" {
				msg = fmt.Sprintf("status %d", delResp.StatusCode)
			}
			t.Fatalf("delete asset %s failed: %s", assetHash, msg)
		}
	}
}

// LoadExampleTemplateData parses template_data from the embedded template_resource_sd.jsonld example.
func LoadExampleTemplateData(t *testing.T) *datatype.JSON {
	t.Helper()

	var presentation map[string]interface{}
	if err := json.Unmarshal(templateResourceSDExample, &presentation); err != nil {
		t.Fatalf("unmarshal template_resource_sd.jsonld failed: %v", err)
	}

	vcs, ok := presentation["verifiableCredential"].([]interface{})
	if !ok || len(vcs) == 0 {
		t.Fatalf("template_resource_sd.jsonld: missing verifiableCredential")
	}
	vc, ok := vcs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("template_resource_sd.jsonld: invalid verifiableCredential entry")
	}
	subject, ok := vc["credentialSubject"].(map[string]interface{})
	if !ok {
		t.Fatalf("template_resource_sd.jsonld: missing credentialSubject")
	}
	rawTemplateData, ok := subject["dcs-template:templateData"].(map[string]interface{})
	if !ok {
		t.Fatalf("template_resource_sd.jsonld: missing dcs-template:templateData")
	}

	templateData, err := datatype.NewJSON(rawTemplateData)
	if err != nil {
		t.Fatalf("marshal example template data failed: %v", err)
	}
	return &templateData
}

// TemplateSeed describes a template resource posted to the Federated Catalogue.
type TemplateSeed struct {
	DID            string
	Version        int
	DocumentNumber string
}

// SeedTemplateResource posts a template asset to the Federated Catalogue.
func SeedTemplateResource(
	t *testing.T,
	ctx context.Context,
	fc *fcclient.FederatedCatalogueClient,
	participantID string,
	did string,
	version int,
	documentNumber string,
	templateType string,
	name string,
	templateData *datatype.JSON,
) TemplateSeed {
	t.Helper()

	now := time.Now().UTC()
	templateJSONLD, err := semanticmapper.BuildTemplateJSONLD(templatedb.ContractTemplate{
		DID:          did,
		Version:      version,
		TemplateType: templateType,
		Name:         &name,
		CreatedAt:    now,
		UpdatedAt:    now,
		TemplateData: templateData,
	}, semanticmapper.DefaultProfile())
	if err != nil {
		t.Fatalf("build template json-ld failed: %v", err)
	}

	payload, err := fcasset.BuildPayload(fcasset.BuildInput{
		TemplateDID:  did,
		Issuer:       participantID,
		ValidFrom:    now,
		TemplateData: templateJSONLD,
	})
	if err != nil {
		t.Fatalf("build template asset payload failed: %v", err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal template asset payload failed: %v", err)
	}

	resp, err := fc.PostRaw(ctx, fcclient.AssetsEndpointPath, nil, fcclient.JSONLDContentType, body)
	if err != nil {
		t.Fatalf("post template asset failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		msg := fc.ExtractErrorMessage(resp.Body)
		if msg == "" {
			msg = fmt.Sprintf("status %d", resp.StatusCode)
		}
		t.Fatalf("post template asset failed: %s", msg)
	}

	return TemplateSeed{
		DID:            did,
		Version:        version,
		DocumentNumber: documentNumber,
	}
}
