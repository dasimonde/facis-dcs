package service

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"

	c2paservice "digital-contracting-service/gen/c2_pa_service"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
)

// DCS-OR-C2PA-008: the public manifest endpoint serves the manifest store of
// the stored PDF of a signed contract as it stands. It never waits for a
// regeneration in flight, so an unauthenticated caller cannot hold a request
// open. A contract without a stored PDF is not found, and so is a contract that
// is not signed: its manifest names its drafts and their reasons (UC-IF-32).
func TestPublicManifestServesTheStoredPDFWithoutWaitingPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	store := artifactstore.New(&reportObjects{blobs: map[string][]byte{}}, &artifactstore.PostgresCEKRepo{DB: database},
		reportAgreement{key}, "did:example:instance", "did:example:instance#agreement", &key.PublicKey)

	// The stored PDF lags the contract: its payload hash names another
	// document, which is what makes an export wait for the regenerator.
	_, err = database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data) VALUES
		('lagging','','Org A','TERMINATED','tpl',1,'{"dcs:clause":"current"}'),
		('draft','','Org A','NEGOTIATION','tpl',1,'{}'),
		('unrendered','','Org A','DRAFT','tpl',1,'{}')`)
	require.NoError(t, err)
	cid, err := store.Put(ctx, artifactstore.ContractScope("lagging"), []byte("%PDF stored"))
	require.NoError(t, err)
	_, err = database.Exec(`UPDATE contracts SET pdf_ipfs_cid = $1, pdf_c2pa_state = 'active', pdf_payload_hash = 'stale', pdf_signed = TRUE WHERE did = 'lagging'`, cid)
	require.NoError(t, err)
	draftCID, err := store.Put(ctx, artifactstore.ContractScope("draft"), []byte("%PDF stored"))
	require.NoError(t, err)
	_, err = database.Exec(`UPDATE contracts SET pdf_ipfs_cid = $1, pdf_c2pa_state = 'draft', pdf_payload_hash = 'h' WHERE did = 'draft'`, draftCID)
	require.NoError(t, err)

	pdfCore := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path != "/manifest/extract" || string(body) != "%PDF stored" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("jumb store"))
	}))
	t.Cleanup(pdfCore.Close)
	svc := NewC2PAService(database, store, &cwepg.PostgresContractRepo{}, pdfcore.New(pdfCore.URL, nil))

	deadline, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, body, err := svc.GetManifest(deadline, &c2paservice.GetManifestPayload{ContractDid: "lagging"})
	require.NoError(t, err)
	served, err := io.ReadAll(body)
	require.NoError(t, err)
	require.Equal(t, "jumb store", string(served))

	history := true
	for _, p := range []*c2paservice.GetManifestPayload{
		{ContractDid: "unrendered"},
		{ContractDid: "draft"},
		{ContractDid: "draft", History: &history},
	} {
		_, _, err = svc.GetManifest(deadline, p)
		var serviceErr *goa.ServiceError
		require.True(t, errors.As(err, &serviceErr) && serviceErr.Name == "not_found", "%s: got %v", p.ContractDid, err)
	}
}
