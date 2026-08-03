package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	dcstodcs2 "digital-contracting-service/internal/dcstodcs"
	pq2 "digital-contracting-service/internal/dcstodcs/db/pg"
	"digital-contracting-service/internal/processauditandcompliance/configattest"

	didservice "digital-contracting-service/gen/did_service"

	genauth "digital-contracting-service/gen/auth"
	c2paservice "digital-contracting-service/gen/c2_pa_service"
	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	keyinventory "digital-contracting-service/gen/key_inventory"
	pdfgeneration "digital-contracting-service/gen/pdf_generation"
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	semantichubgen "digital-contracting-service/gen/semantic_hub"
	signaturemanagement "digital-contracting-service/gen/signature_management"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/auth"
	pg "digital-contracting-service/internal/auth/db/pq"
	"digital-contracting-service/internal/auth/machineidentity"
	"digital-contracting-service/internal/auth/oid4vp"
	oid4vprequest "digital-contracting-service/internal/auth/oid4vp/request"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/db/pq"
	"digital-contracting-service/internal/base/envelope"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/federation"
	"digital-contracting-service/internal/base/hsm"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/tsa"
	"digital-contracting-service/internal/base/validation"
	contractworkflowengine2 "digital-contracting-service/internal/contractworkflowengine"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	cwerepo "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/contractworkflowengine/deployevent"
	"digital-contracting-service/internal/middleware"
	pdfevent "digital-contracting-service/internal/pdfgeneration/event"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/pdfgeneration/statuspublication"
	"digital-contracting-service/internal/processauditandcompliance/auditexecutor"
	pacrepo "digital-contracting-service/internal/processauditandcompliance/db/pg"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
	"digital-contracting-service/internal/semantichub"
	"digital-contracting-service/internal/service"
	smrepo "digital-contracting-service/internal/signingmanagement/db/pg"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	tplrepo "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/webhookplatform"
	"digital-contracting-service/migrations"
	"digital-contracting-service/migrations/fcschemas"

	"github.com/jmoiron/sqlx"
	"github.com/nats-io/nats.go"
	"goa.design/clue/debug"
	"goa.design/clue/log"
)

// computeListenURL derives the HTTP listen address from the same flags/env
// var handleHTTPServer's caller used to compute inline just before binding.
// Hoisted out so a bootstrap server (see startBootstrapServer) can claim the
// same address immediately at process start, before the PKCS#11 token is
// necessarily provisioned.
func computeListenURL(ctx context.Context, hostF, domainF, httpPortF string, secureF bool) *url.URL {
	if hostF != "local" {
		log.Fatal(ctx, fmt.Errorf("invalid host argument: %q (valid hosts: local)", hostF))
	}
	address := "http://0.0.0.0:8991"
	if os.Getenv("DCS_BACKEND_PORT") != "" {
		address = fmt.Sprintf("http://0.0.0.0:%s", os.Getenv("DCS_BACKEND_PORT"))
	}
	u, err := url.Parse(address)
	if err != nil {
		log.Fatalf(ctx, err, "invalid URL %#v\n", address)
	}
	if secureF {
		u.Scheme = "https"
	}
	if domainF != "" {
		u.Host = domainF
	}
	if httpPortF != "" {
		h, _, err := net.SplitHostPort(u.Host)
		if err != nil {
			log.Fatalf(ctx, err, "invalid URL %#v\n", u.Host)
		}
		u.Host = net.JoinHostPort(h, httpPortF)
	} else if u.Port() == "" {
		u.Host = net.JoinHostPort(u.Host, "80")
	}
	return u
}

// startBootstrapServer claims the service's listen address immediately and
// reports 503 on /readyz until initialization is complete. This lets
// Kubernetes distinguish a running bootstrap process from a ready DCS while
// the HSM, FC functional gate, and schema synchronization are pending.
//
// The chart's supported deployment path intentionally runs Helm without
// --wait: the HSM provisioner is a post-install hook and supplies material
// needed before this endpoint can turn ready. deploy.sh waits for the backend
// rollout only after Helm has executed that hook.
func startBootstrapServer(ctx context.Context, u *url.URL) *http.Server {
	srv := &http.Server{
		Addr:              u.Host,
		Handler:           bootstrapHTTPHandler(),
		ReadHeaderTimeout: time.Second * 60,
	}
	go func() {
		log.Printf(ctx, "bootstrap HTTP server listening on %q (initializing)", u.Host)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf(ctx, err, "bootstrap HTTP server error")
		}
	}()
	return srv
}

// openHSMWithRetry retries hsm.Open until it succeeds, instead of the
// immediate log.Fatalf every other HSM/config error in main() uses. This is
// the one call in the chain that's expected to fail transiently at fresh
// install/upgrade (see startBootstrapServer's comment) — hsm-provision.sh
// creates the token and all five keys in one run, so once Open succeeds the
// downstream hsmClient.Signer(label) calls are not expected to need the same
// treatment.
func openHSMWithRetry(ctx context.Context) *hsm.HSM {
	const interval = 5 * time.Second
	attempt := 0
	for {
		attempt++
		hsmClient, err := hsm.Open(hsm.ConfigFromEnv())
		if err == nil {
			return hsmClient
		}
		if attempt == 1 || attempt%12 == 0 { // log immediately, then once/minute
			log.Printf(ctx, "waiting for PKCS#11 token (attempt %d): %v", attempt, err)
		}
		time.Sleep(interval)
	}
}

func main() {
	var (
		hostF     = flag.String("host", "local", "Server host (valid values: local)")
		domainF   = flag.String("domain", "", "Host domain name (overrides host domain specified in service design)")
		httpPortF = flag.String("http-port", "", "HTTP port (overrides host HTTP port specified in service design)")
		secureF   = flag.Bool("secure", false, "Use secure scheme (https or grpcs)")
		dbgF      = flag.Bool("debug", false, "Log request and response bodies")
		envF      = flag.String("env", "", "Set environment file for the service")
	)
	flag.Parse()

	if envF != nil && *envF != "" {
		if err := loadDotenvFile(*envF); err != nil {
			_, err := fmt.Fprintf(os.Stderr, "startup configuration error: %v\n", err)
			if err != nil {
				return
			}
			os.Exit(1)
		}
	} else {
		if err := loadDotenvIfPresent(); err != nil {
			_, err := fmt.Fprintf(os.Stderr, "startup configuration error: %v\n", err)
			if err != nil {
				return
			}
			os.Exit(1)
		}
	}

	format := log.FormatJSON
	if log.IsTerminal() {
		format = log.FormatTerminal
	}
	ctx := log.Context(context.Background(), log.WithFormat(format))
	if *dbgF {
		ctx = log.Context(ctx, log.WithDebug())
		log.Debugf(ctx, "debug logs enabled")
	}
	log.Print(ctx, log.KV{K: "http-port", V: *httpPortF})

	db, err := NewDatabaseConnection()
	if err != nil {
		log.Fatalf(ctx, err, "Could not connect to database")
	}
	defer func(db *sqlx.DB) {
		err := db.Close()
		if err != nil {
			fmt.Printf("could not close database connection: %v\n", err)
		}
	}(db)

	log.Printf(ctx, "Connecting to database")

	// Run database migrations
	if err := migrations.Run(db); err != nil {
		log.Fatalf(ctx, err, "Could not run database migrations")
		os.Exit(1)
	}

	// DCS_PUBLIC_URL is the base of every absolute IRI a produced document
	// carries (@context, sh:shapesGraph, C2PA remote manifests) — these must
	// dereference for external consumers.
	if strings.TrimSpace(os.Getenv("DCS_PUBLIC_URL")) == "" {
		log.Fatalf(ctx, errors.New("dcs configuration missing"), "DCS_PUBLIC_URL must be set: produced documents carry absolute, resolvable IRIs based on it")
	}

	// Seed the Semantic Hub genesis schemas (JSON-LD context, SHACL shapes,
	// validation profile) and anchor document production to the active
	// versions. The SemanticHub service re-runs the anchor refresh after
	// every activation/rollback.
	if err := semantichub.Seed(ctx, db); err != nil {
		log.Fatalf(ctx, err, "Could not seed the Semantic Hub genesis schemas")
	}
	if err := service.RefreshValidationAnchors(ctx, db); err != nil {
		log.Fatalf(ctx, err, "Could not anchor validation to the Semantic Hub's active schemas")
	}

	validation.SetShapeSource(semantichub.HubShapeSource{DB: db})

	// Open the PKCS#11 token that holds every private key (DCS-IR-HI-01) — no
	// software fallback once open, but opening itself is retried rather than
	// immediately fatal: a fresh install/upgrade's hsm-provision Job may not
	// have run yet (see openHSMWithRetry's doc comment). The bootstrap server
	// claims the listen address now while /readyz remains unavailable during
	// retries; handleHTTPServer takes over the same address once ready.
	listenURL := computeListenURL(ctx, *hostF, *domainF, *httpPortF, *secureF)
	bootstrapSrv := startBootstrapServer(ctx, listenURL)
	hsmClient := openHSMWithRetry(ctx)
	defer func() {
		if err := hsmClient.Close(); err != nil {
			log.Errorf(ctx, err, "Could not close PKCS#11 token")
		}
	}()

	didFilePath := os.Getenv("DCS_DID")

	didSigner, err := hsmClient.Signer(hsm.KeyLabelDID())
	if err != nil {
		log.Fatalf(ctx, err, "Could not load HSM DID signing key")
	}

	log.Printf(ctx, "Reading did.json")
	didDocument, err := identity.NewDIDDocument(didFilePath, didSigner)
	if err != nil {
		log.Fatalf(ctx, err, "Could not read did document")
	}

	var euTrustPool *identity.EUTrustPool
	if base.GetEnvOrDefault("DCS_FORCE_EIDAS_CERT", false) {
		log.Printf(ctx, "Start building EU trust pool")
		trustPool := identity.NewEUTrustPool()
		if err := trustPool.Refresh(ctx); err != nil {
			log.Fatalf(ctx, err, "Building EU trust pool")
		}
		count, _, errs := trustPool.Stats()
		log.Printf(ctx, "EU trust pool ready: %d certificates (%d lists skipped)", count, len(errs))

		// Keep it fresh in the background.
		go trustPool.StartAutoRefresh(ctx, identity.DefaultRefreshInterval)

		euTrustPool = trustPool
	}

	err = didDocument.VerifyEIDASCertificate(euTrustPool)
	if err != nil {
		log.Fatalf(ctx, err, "Could not verify certificate")
	}

	// Initialize OIDC validator and JWT authenticator.
	authCfg, err := loadAuthConfig(ctx)
	if err != nil {
		log.Fatalf(ctx, err, "Could not load auth config")
	}

	// Every OpenID4VP request object this deployment issues — login, PID, the
	// signing ceremony's identity presentation, and the Document-Retrieval
	// request — is signed with the DCS's own DID/hostname certificate chain
	// (already verified, just above, to carry a SAN matching its hostname). A
	// wallet resolves trust from the leaf certificate's SAN named by the
	// x509_san_dns client identifier; a bare jwk header proves possession of a
	// key it has no reason to trust and anchors to nothing.
	requestSigner, err := oid4vprequest.NewX5CSigner(didDocument)
	if err != nil {
		log.Fatalf(ctx, err, "Could not build OID4VP request signer")
	}
	// The wallet is handed an OpenID4VP client identifier — prefix and value —
	// not the Hydra OAuth client id: an unprefixed value means the
	// "pre-registered" prefix, which a wallet outside a pre-agreed federation
	// refuses before it looks at any credential.
	oid4vpClientID, err := requestSigner.ClientID()
	if err != nil {
		log.Fatalf(ctx, err, "Could not resolve OID4VP client_id")
	}

	authCfg.RequestSigner = requestSigner
	authCfg.OID4VPClientID = oid4vpClientID
	// Machine callers are resolved from the registry at request time, so an
	// identity can be added, disabled or rotated without a redeploy (ADR-27).
	// DCS_SYSTEM_CLIENTS remains a declarative seed for the callers a
	// deployment must have before anyone logs in to create them.
	machineIdentities := machineidentity.NewPostgresRepo(db)
	seeded, err := loadSystemClients()
	if err != nil {
		log.Fatalf(ctx, err, "Invalid system client configuration")
	}
	if err := seedMachineIdentities(ctx, machineIdentities, seeded); err != nil {
		log.Fatalf(ctx, err, "Could not seed the configured system clients")
	}
	hydraJWTValidator, err := middleware.NewHydraJWTValidator(ctx, middleware.HydraJWTConfig{
		PublicIssuerURL:   authCfg.Hydra.PublicIssuerURL(),
		InternalIssuerURL: authCfg.Hydra.InternalIssuerURL(),
		ClientID:          authCfg.Hydra.ClientID(),
		SystemClients:     machineIdentities,
	})
	if err != nil {
		log.Fatalf(ctx, err, "Failed to initialize Hydra JWT validator")
	}

	// Initialize IPFS client
	ipfsTenantBaseURL := os.Getenv("IPFS_TENANT_BASE_URL")
	mfsBaseURL := os.Getenv("IPFS_MFS_BASE_URL")
	if ipfsTenantBaseURL == "" || mfsBaseURL == "" {
		log.Fatalf(ctx, nil, "IPFS configuration missing: IPFS_TENANT_BASE_URL and IPFS_MFS_BASE_URL environment variables must be specified")
	}
	ipfsAPIClient := ipfs.NewClient(ipfsTenantBaseURL, mfsBaseURL)

	aAttemptRepo := &pg.PostgresAccessAttemptRepo{}
	lockRepo := &pg.PostgresIPLockoutRepo{}
	jwtAuth := auth.NewJWTAuthenticator(hydraJWTValidator, db, aAttemptRepo, lockRepo)

	ctRepo := tplrepo.PostgresContractTemplateRepo{}
	ctRTRepo := tplrepo.PostgresReviewTaskRepo{}
	ctATRepo := tplrepo.PostgresApprovalTaskRepo{}

	cweRepo := cwerepo.PostgresContractRepo{}
	cweRTRepo := cwerepo.PostgresReviewTaskRepo{}
	cweATRepo := cwerepo.PostgresApprovalTaskRepo{}
	cweNTRepo := cwerepo.PostgresNegotiationTaskRepo{}
	cweNRepo := cwerepo.PostgresNegotiationRepo{}
	cweCTRepo := cwerepo.PostgresContractTemplateRepo{}
	cweCronJob := contractworkflowengine2.CronJob{DB: db, CRepo: &cweRepo}
	cweCronJob.Start(ctx, db)

	pacRiskRepo := pacrepo.PostgresRiskFindingRepo{}

	aRepo := pq.PostgresAuditTrailRepository{}

	tsaURL := os.Getenv("TSA_URL")
	if tsaURL == "" {
		log.Fatalf(ctx, nil, "TSA_URL is not set")
	}
	tsaClient, err := tsa.NewClient(tsaURL)
	if err != nil {
		log.Fatalf(ctx, err, "failed to initialize TSA client")
	}

	// Connect to NATS (use NATS_URL env var or default)
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = nats.DefaultURL
	}

	cepPubClient, err := event.NewNatsPubClient(conf.EventBusTopic(), natsURL)
	if err != nil {
		log.Fatalf(ctx, err, "Could not connect to events bus")
	}
	defer func(client *event.CloudEventPubClient) {
		err := client.Close()
		if err != nil {
			log.Errorf(ctx, err, "Could not close cloud event bus client")
		}
	}(cepPubClient)

	did, err := didDocument.GetID()
	if err != nil {
		log.Fatalf(ctx, err, "could not read DID")
	}
	// A login credential attests authority for one organization; this instance
	// only grants access to itself.
	authCfg.ParticipantDID = did

	// Encrypting artifact store (DCS-NFR-SEC-14): every IPFS artifact is
	// encrypted per scope; CEKs are wrapped to the instance's keyAgreement key.
	// The dcs-ecdh key is a required dependency: a startup wrap/unwrap self-test
	// proves the served did.json keyAgreement key and the HSM key agree.
	ecdhLabel := hsm.KeyLabelECDH()
	keyAgreementMethod, err := didDocument.OwnKeyAgreementMethod(ecdhLabel)
	if err != nil {
		log.Fatalf(ctx, err, "did.json carries no usable keyAgreement method for %s", ecdhLabel)
	}
	keyAgreementPub, err := keyAgreementMethod.ECPublicKey()
	if err != nil {
		log.Fatalf(ctx, err, "did.json keyAgreement method %s carries no usable P-256 key", keyAgreementMethod.ID)
	}
	hsmAgreement := envelope.HSMKeyAgreement(ecdhLabel)
	if testCEK, err := envelope.NewCEK(); err != nil {
		log.Fatalf(ctx, err, "could not draw CEK for the key-agreement self-test")
	} else if wrapped, err := envelope.Wrap(testCEK, keyAgreementMethod.ID, keyAgreementPub); err != nil {
		log.Fatalf(ctx, err, "could not wrap the self-test CEK to the %s key", ecdhLabel)
	} else if unwrapped, err := envelope.Unwrap(wrapped, hsmAgreement); err != nil || !bytes.Equal(unwrapped, testCEK) {
		log.Fatalf(ctx, err, "HSM key %s cannot unwrap what did.json's keyAgreement key wraps — key mismatch or missing derive capability", ecdhLabel)
	}
	artifactStore := artifactstore.New(ipfsAPIClient, &artifactstore.PostgresCEKRepo{DB: db}, hsmAgreement, did, keyAgreementMethod.ID, keyAgreementPub)

	// Startup config integrity verification (DCS-NFR-SEC-04): hash the
	// security-critical mounted config files, enforce any operator pins, and
	// record the attestation in the audit outbox. A pin mismatch or an
	// unreadable configured file aborts startup.
	if err := configattest.Attest(ctx, db, did, map[string]string{
		"did-document":      os.Getenv("DCS_DID"),
		"oid4vp-trust-data": os.Getenv("OID4VP_TRUST_DATA_PATH"),
		"x5c-trust-anchors": os.Getenv("OID4VP_X5C_TRUST_ANCHORS_PATH"),
		// The authorization policy outranks the trust document: a rule granting
		// everything overrides every entry in it. Attesting the document while
		// leaving the policy unpinned would put the pinned file under the
		// unpinned one. Empty unless a deployment supplies its own — the built-in
		// policy is compiled into the binary and covered by the image digest.
		oid4vp.TrustPolicyAttestationKey: oid4vp.TrustPolicyPath(),
	}, os.Getenv("DCS_CONFIG_SHA256_PINS")); err != nil {
		log.Fatalf(ctx, err, "Config integrity verification failed")
	}

	outboxProcessor := event.OutboxProcessor{
		DB:           db,
		CEPPubClient: cepPubClient,
		IPFSClient:   ipfsAPIClient,
		Artifacts:    artifactStore,
		ARepo:        &aRepo,
		TSAClient:    tsaClient,
	}
	err = outboxProcessor.Start(ctx)
	if err != nil {
		log.Fatalf(ctx, err, "failed to start outbox processor")
	}

	cepSubClient, err := event.NewNatsSubClient(conf.EventBusTopic(), natsURL)
	if err != nil {
		log.Fatalf(ctx, err, "Could not connect to events bus")
	}
	defer func(client *event.CloudEventSubClient) {
		err := client.Close()
		if err != nil {
			log.Errorf(ctx, err, "Could not close cloud event bus client")
		}
	}(cepSubClient)

	syncRepo := pq2.PostgresSyncRepository{}
	// Federation trust gate (ADR-19): agreement credential verification + the
	// local policy endpoint, consulted on both the outbound (shipContractPDF,
	// here) and inbound (PostPdf, service.NewDcsToDcs below) paths.
	trustGate := dcstodcs2.TrustGate{PDPURL: os.Getenv("DCS_TRUST_PDP_URL")}
	// Mutual Power-of-Attorney binding (ADR-31): the synchronizer ships the
	// credential behind each signature this instance applied, and the gate
	// verifies the credential a counterparty ships back.
	ceremonyPoAs := &dcstodcs2.CeremonyPoAs{DB: db, CeremonyRepo: &smrepo.PostgresCeremonyRepo{}}
	counterpartyPoAGate := dcstodcs2.CounterpartyPoAGate{Trust: authCfg.Trust}
	dcsToDcsSynchronizer := dcstodcs2.DCSToDCSSynchronizer{
		DB:               db,
		CRepo:            &cweRepo,
		SRepo:            &syncRepo,
		Artifacts:        artifactStore,
		DIDDocument:      *didDocument,
		TrustGate:        trustGate,
		PoAs:             ceremonyPoAs,
		SettlementSender: dcstodcs2.HTTPSettlementSender{},
	}
	dcsToDcsSynchronizer.StartSynchronizerJob(ctx, cepSubClient)

	// Contract erasure (DCS-NFR-COMP-03, DCS-NFR-SEC-13): local CEK shredding
	// with KEY_SHREDDED audit events plus the peer erase handshake, retried
	// from contract_erasures on the sync-fail interval.
	cekRepo := &artifactstore.PostgresCEKRepo{DB: db}
	eraseRepo := &pq2.PostgresEraseRepository{DB: db}
	shredder := &dcstodcs2.AuditedShredder{DB: db, Keys: cekRepo, Artifacts: artifactStore}
	contractEraser := &dcstodcs2.ContractEraser{
		DIDDocument: *didDocument,
		Shredder:    shredder,
		Parties:     &dcstodcs2.DBContractParties{DB: db, CRepo: &cweRepo},
		ERepo:       eraseRepo,
		Sender:      dcstodcs2.HTTPPeerEraseSender{},
	}
	contractEraser.StartEraseRetryJob(ctx, conf.SyncFailCronJobTimeOut())

	if os.Getenv("DCS_DEBUG_EVENTING") == "true" {
		event.StartEventLogger(ctx, cepSubClient)
	}

	auditTrailReader := base.AuditTrailReader{
		IPFSClient: ipfsAPIClient,
		Artifacts:  artifactStore,
		ARepo:      &aRepo,
	}

	archiveNotaryURL := strings.TrimSpace(os.Getenv("ORCE_ARCHIVE_NOTARY_URL"))
	var archiveNotaryClient cwecommand.ArchiveNotary
	if archiveNotaryURL != "" {
		archiveNotaryClient = cwecommand.NewHTTPArchiveNotaryClient(archiveNotaryURL, os.Getenv("ORCE_ARCHIVE_AUDIT_LOG_BEARER_TOKEN"))
	}

	// Contract deployment (UC-05-01): a contract designates a registered target
	// system (ADR-25) and the client dispatches to that entry's endpoint. The
	// target's own callback (POST /contract/deployment/callback) remains the
	// authoritative signal of a successful deployment; a failed outbound call is
	// recorded on the deployment row so the compliance monitor can alert on it.
	cweDeploymentRepo := &cwerepo.PostgresDeploymentRepo{}
	cweTargetRepo := cwerepo.PostgresContractTargetRepo{}

	// Target systems declared in deployment configuration (ADR-25). A fresh
	// install has an empty registry, so without this nothing can be deployed
	// until somebody opens the admin UI — including in a test cluster that is
	// recreated every run. Entries already present are left untouched, so an
	// administrator who repoints one is not overruled by the next restart.
	if seedPath := strings.TrimSpace(os.Getenv("CONTRACT_TARGETS_FILE")); seedPath != "" {
		raw, err := os.ReadFile(seedPath)
		if err != nil {
			log.Fatalf(ctx, err, "could not read CONTRACT_TARGETS_FILE %s", seedPath)
		}
		entries, err := cwecommand.ParseSeedTargets(raw)
		if err != nil {
			log.Fatalf(ctx, err, "invalid contract target configuration in %s", seedPath)
		}
		seeded, err := cwecommand.SeedContractTargets(ctx, db, &cweTargetRepo, entries)
		if err != nil {
			log.Fatalf(ctx, err, "could not register the configured contract targets")
		}
		log.Printf(ctx, "contract target registry: %d of %d configured targets registered", seeded, len(entries))
	}
	// One client for every registered target: the endpoint travels with each
	// dispatch now that a contract names its own destination (ADR-25).
	contractTargetClient := cwecommand.NewHTTPContractTargetClient()

	// Initialize the Federated Catalogue client.
	fcURL := os.Getenv("FEDERATED_CATALOGUE_API_URL")
	fcClientID := os.Getenv("FEDERATED_CATALOGUE_CLIENT_ID")
	fcClientSecret := os.Getenv("FEDERATED_CATALOGUE_CLIENT_SECRET")
	fcRealmURL := strings.TrimSpace(os.Getenv("FC_KEYCLOAK_REALM_URL"))
	var templateCatalogueClient *fcclient.FederatedCatalogueClient
	if fcURL != "" || fcClientID != "" || fcClientSecret != "" || fcRealmURL != "" {
		var missing []string
		for name, value := range map[string]string{
			"FEDERATED_CATALOGUE_API_URL":       fcURL,
			"FEDERATED_CATALOGUE_CLIENT_ID":     fcClientID,
			"FEDERATED_CATALOGUE_CLIENT_SECRET": fcClientSecret,
			"FC_KEYCLOAK_REALM_URL":             fcRealmURL,
		} {
			if strings.TrimSpace(value) == "" {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			slices.Sort(missing)
			log.Fatalf(ctx, nil, "incomplete Federated Catalogue configuration; missing %s", strings.Join(missing, ", "))
		}
		templateCatalogueClient, err = fcclient.NewFederatedCatalogueClient(fcclient.Config{
			APIURL:           fcURL,
			KeycloakRealmURL: fcRealmURL,
			ClientID:         fcClientID,
			ClientSecret:     fcClientSecret,
		})
		if err != nil {
			log.Fatalf(ctx, err, "failed to initialize Federated Catalogue client")
		}
		requireFCHealth := strings.EqualFold(strings.TrimSpace(os.Getenv("FEDERATED_CATALOGUE_REQUIRE_NATIVE_HEALTH")), "true")
		if err := templateCatalogueClient.CheckReadiness(ctx, requireFCHealth); err != nil {
			log.Fatalf(ctx, err, "federated catalogue readiness gate failed")
		}
		if err := fcschemas.Sync(ctx, templateCatalogueClient); err != nil {
			log.Fatalf(ctx, err, "failed to sync federated catalogue schemas")
		}
	}

	// Initialize the webhook platform (ORCE integration).
	webhookStore := webhookplatform.NewSubscriptionStore()
	webhookDispatcher := webhookplatform.NewDispatcher(webhookStore)
	webhookPlatform := webhookplatform.New(
		webhookStore,
		webhookDispatcher,
		func(ctx context.Context, token string) (string, error) {
			info, err := hydraJWTValidator.ValidateToken(ctx, token)
			if err != nil {
				return "", err
			}
			return info.ParticipantDID, nil
		},
		nil,
	)

	// Start the NATS→Webhook bridge: automatically fans out to all registered
	// webhook subscribers whenever a DCS lifecycle event fires on the event bus.
	webhookSubClient, err := event.NewNatsSubClient(conf.EventBusTopic(), natsURL)
	if err != nil {
		log.Fatalf(ctx, err, "Could not create webhook NATS subscriber")
	}
	defer func(webhookSubClient *event.CloudEventSubClient) {
		err := webhookSubClient.Close()
		if err != nil {
			log.Errorf(ctx, err, "failed to close webhook subscriber")
		}
	}(webhookSubClient)
	go func() {
		if err := webhookplatform.StartNATSBridge(webhookSubClient, webhookDispatcher); err != nil {
			log.Fatalf(ctx, err, "Could not start webhook NATS bridge")
		}
	}()

	// Sign contract-lifecycle VCs (DCS-OR-C2PA-004) with the HSM VC key,
	// producing an ecdsa-rdfc-2019 Data Integrity proof.
	issuerDID := os.Getenv("ISSUER_DID")
	vcKeyLabel := hsm.KeyLabelVC()
	vcHSMSigner, err := hsmClient.Signer(vcKeyLabel)
	if err != nil {
		log.Fatalf(ctx, err, "Could not load HSM VC signing key")
	}
	vcSigner := provenance.NewHSMVCSigner(vcHSMSigner, vcKeyLabel)

	// Sign COSE Sig_structure bytes for pdf-core's C2PA manifests with the HSM
	// C2PA key. pdf-core prepares the Sig_structures; the DCS signs them in-process
	// via the pdf-core client and posts them back for embedding (pdf-core is keyless).
	c2paSigner, err := hsmClient.Signer(hsm.KeyLabelC2PA())
	if err != nil {
		log.Fatalf(ctx, err, "Could not load HSM C2PA signing key")
	}

	// The status list for the credentials this deployment issues — the contract
	// lifecycle credential inside every generated PDF and the signing summary
	// that travels beside a contract (DCS-OR-C2PA-005, ADR-34). This deployment
	// mints them, so it publishes their revocation status: allocation and bit in
	// its own database, served and signed here with the same key and certificate
	// chain a verifier already has to trust to accept the credential itself.
	statusListIssuerURL, err := provenance.StatusListIssuerURL(os.Getenv("DCS_PUBLIC_URL"))
	if err != nil {
		log.Fatalf(ctx, err, "status list configuration error")
	}
	// The list new revocation entries are allocated in. It only moves when list 1
	// fills up, and then only after the operator has registered the successor in
	// status_list_cursors — allocation hard-fails on an unregistered list rather
	// than handing out an entry nothing serves.
	statusListID := provenance.DefaultListID
	if raw := strings.TrimSpace(os.Getenv("STATUSLIST_LIST_ID")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			log.Fatalf(ctx, err, "STATUSLIST_LIST_ID must be a positive list id, got %q", raw)
		}
		statusListID = parsed
	}
	statusListURI := func(listID int) string {
		return provenance.StatusListURI(statusListIssuerURL, listID)
	}
	statusListRevocations := &provenance.PostgresStatusListRevocations{DB: db}
	statusListPublisher := provenance.NewDCSStatusListPublisher(
		statusListURI,
		provenance.NewPostgresStatusListAllocator(db, statusListID),
		statusListRevocations)
	go (&statuspublication.Worker{DB: db, Publisher: statusListPublisher}).Run(ctx)

	// The served token. Its chain is the C2PA one (same PKCS#11 key), and the
	// leaf has to name statusListIssuerURL or a verifier — ours included —
	// refuses the list.
	issuerX5Chain, err := provenance.LoadX5Chain(os.Getenv(issuerX5ChainPathEnv))
	if err != nil {
		log.Fatalf(ctx, err, "%s must point at this deployment's certificate chain: the status list it serves carries it", issuerX5ChainPathEnv)
	}
	// The same chain in its PEM form, for pdf-core. pdf-core keeps no signing
	// material: it prepares the COSE Sig_structures, this process signs them with
	// the dcs-c2pa key, and the certificate naming that key travels with each
	// request rather than being configured into a renderer several instances share.
	issuerX5ChainPEM, err := os.ReadFile(os.Getenv(issuerX5ChainPathEnv))
	if err != nil {
		log.Fatalf(ctx, err, "read %s: pdf-core signs this instance's manifests under this chain", issuerX5ChainPathEnv)
	}
	statusListSigner := &provenance.StatusListSigner{
		Issuer:      statusListIssuerURL,
		ListURI:     statusListURI,
		ListID:      statusListID,
		Chain:       issuerX5Chain,
		Signer:      c2paSigner,
		Revocations: statusListRevocations,
		Size:        provenance.ListSize,
	}

	// Initialize pdf-core client (PDF rendering + C2PA provenance microservice).
	pdfCoreURL := os.Getenv("PDF_CORE_URL")
	if pdfCoreURL == "" {
		log.Fatalf(ctx, nil, "PDF_CORE_URL is required")
	}
	if err := probeHTTPUntilReady(3*time.Minute, func() error {
		return probeHTTP(pdfCoreURL + "/version")
	}); err != nil {
		log.Fatalf(ctx, err, "pdf-core not reachable at %s", pdfCoreURL)
	}
	pdfCoreClient := pdfcore.NewWithAuthority(pdfCoreURL, func(sigStructure []byte) ([]byte, error) {
		return hsm.SignES256(c2paSigner, sigStructure)
	}, issuerDID).WithSigningChain(issuerX5ChainPEM)

	smCRepo := smrepo.PostgresContractRepo{
		Artifacts: artifactStore,
		PDFCore:   pdfCoreClient,
	}

	// This instance's federation agreement credential (ADR-19): issuer = this
	// instance's own DID, termsOfUse names the embedded federation rules document
	// by its public policy URL and hash. Minted on first fetch and re-minted
	// before its bounded validUntil lapses, so a long-running process never
	// publishes an expired one.
	rulesPolicyURL := federation.RulesPolicyURL(os.Getenv("DCS_PUBLIC_URL"))
	agreementCredential := &federation.CredentialIssuer{
		Signer:    vcSigner,
		IssuerDID: issuerDID,
		RulesURL:  rulesPolicyURL,
	}
	if _, err := agreementCredential.Current(ctx); err != nil {
		log.Fatalf(ctx, err, "failed to build federation agreement credential")
	}

	didService, err := service.NewDIDService(*didDocument, agreementCredential, federation.Rules())
	if err != nil {
		log.Fatalf(ctx, err, "failed to create did service")
	}

	// Initialize the service.
	var (
		authSvc                         genauth.Service
		contractStorageArchiveSvc       contractstoragearchive.Service
		contractWorkflowEngineSvc       contractworkflowengine.Service
		dcsToDcsSvc                     dcstodcs.Service
		pdfGenerationSvc                pdfgeneration.Service
		processAuditAndComplianceSvc    processauditandcompliance.Service
		signatureManagementSvc          signaturemanagement.Service
		templateCatalogueIntegrationSvc templatecatalogueintegration.Service
		templateRepositorySvc           templaterepository.Service
		didSrv                          didservice.Service
		c2paSvc                         c2paservice.Service
		semanticHubSvc                  semantichubgen.Service
		keyInventorySvc                 keyinventory.Service
		workflowGateCoordinator         *workflowgate.Coordinator
	)
	{
		auditExecutorTimeout := 10 * time.Second
		if configured := strings.TrimSpace(os.Getenv("PAC_AUDIT_EXECUTOR_TIMEOUT")); configured != "" {
			parsed, parseErr := time.ParseDuration(configured)
			if parseErr != nil || parsed <= 0 {
				log.Fatalf(ctx, parseErr, "invalid PAC_AUDIT_EXECUTOR_TIMEOUT %q", configured)
			}
			auditExecutorTimeout = parsed
		}
		auditExecutorClient, clientErr := auditexecutor.NewHTTPClient(
			os.Getenv("PAC_AUDIT_EXECUTOR_URL"),
			os.Getenv("PAC_AUDIT_EXECUTOR_BEARER_TOKEN"),
			auditExecutorTimeout,
		)
		if clientErr != nil {
			log.Fatalf(ctx, clientErr, "audit executor configuration is invalid")
		}
		workflowGateTimeout := 10 * time.Second
		if configured := strings.TrimSpace(os.Getenv("PAC_WORKFLOW_GATE_EXECUTOR_TIMEOUT")); configured != "" {
			parsed, parseErr := time.ParseDuration(configured)
			if parseErr != nil || parsed <= 0 {
				log.Fatalf(ctx, parseErr, "invalid PAC_WORKFLOW_GATE_EXECUTOR_TIMEOUT %q", configured)
			}
			workflowGateTimeout = parsed
		}
		workflowGateClient, clientErr := workflowgate.NewHTTPClient(
			os.Getenv("PAC_WORKFLOW_GATE_EXECUTOR_URL"),
			os.Getenv("PAC_WORKFLOW_GATE_EXECUTOR_BEARER_TOKEN"),
			workflowGateTimeout,
		)
		if clientErr != nil {
			log.Fatalf(ctx, clientErr, "workflow-gate executor configuration is invalid")
		}
		workflowGateCoordinator = &workflowgate.Coordinator{DB: db, Client: workflowGateClient}

		presentationRepo := pg.NewPostgresPresentationAttemptRepo(db)
		authSvc, err = service.NewAuth(db, presentationRepo, authCfg)
		if err != nil {
			log.Fatalf(ctx, err, "auth service init failed")
		}

		contractStorageArchiveSvc = service.NewContractStorageArchive(db, jwtAuth, &cweRepo, *didDocument, auditTrailReader, ipfsAPIClient, contractEraser, cekRepo, eraseRepo)
		contractWorkflowEngineSvc = service.NewContractWorkflowEngine(db, jwtAuth, &cweRepo, &cweRTRepo, &cweATRepo, &cweNTRepo, &cweNRepo, &cweCTRepo, &syncRepo, euTrustPool, templateCatalogueClient, auditTrailReader, *didDocument, archiveNotaryClient, tsaClient, cweDeploymentRepo, &cweTargetRepo, contractTargetClient,
			workflowGateCoordinator, machineIdentities, authCfg.Hydra, authCfg.Hydra.PublicIssuerURL())
		dcsToDcsSvc = service.NewDcsToDcs(db, jwtAuth, &cweRepo, &cweRTRepo, &cweATRepo, &cweNTRepo, &cweNRepo, &cweCTRepo, &syncRepo, euTrustPool, *didDocument, artifactStore, pdfCoreClient, trustGate, counterpartyPoAGate, shredder)
		// The credentials this instance issued verify against its own published
		// document without a round trip; a peer's lifecycle or signing-summary
		// credential — carried inside a verbatim-stored inbound PDF — resolves to
		// the peer's did:web.
		credentialVerifier := &provenance.CredentialVerifier{Own: didDocument}
		// The revocation state of those same credentials, read from the status
		// list each one names — ours through the endpoint below, a peer's through
		// theirs — and only once that list's own signature verified against the
		// configured anchors (ADR-34). Shares the verifier the OID4VP login path
		// uses, so an issuer trusted for a presented credential and an issuer
		// trusted for a provenance credential cannot become two different sets.
		credentialStatusVerifier := provenance.NewCredentialStatusVerifier(oid4vp.StatusListVerifier())
		pdfGenerationSvc = service.NewPDFGeneration(db, jwtAuth, artifactStore, &cweRepo, &ctRepo, &smCRepo, pdfCoreClient, issuerDID, provenance.NewLocalVCIssuer(vcSigner, issuerDID, statusListPublisher), did, credentialVerifier, credentialStatusVerifier)
		c2paSvc = service.NewC2PAService(db, artifactStore, &cweRepo, pdfCoreClient, issuerDID, provenance.NewLocalVCIssuer(vcSigner, issuerDID, statusListPublisher))
		processAuditAndComplianceSvc = service.NewProcessAuditAndCompliance(db, jwtAuth, auditTrailReader, &ctRepo, &cweRepo, &cweATRepo, &pacRiskRepo, auditExecutorClient, workflowGateCoordinator)
		signatureManagementSvc = service.NewSignatureManagement(db, jwtAuth, &smCRepo, &smrepo.PostgresCeremonyRepo{}, auditTrailReader, vcSigner, issuerDID, artifactStore, pdfCoreClient, &cweRepo, archiveNotaryClient, tsaClient, provenance.NewLocalVCIssuer(vcSigner, issuerDID, statusListPublisher), workflowGateCoordinator, requestSigner, oid4vpClientID, authCfg.PublicAPIBase, authCfg.PIDDCQLQuery, authCfg.DCQLQuery, authCfg.Trust, credentialVerifier, credentialStatusVerifier, statusListPublisher, *didDocument, &syncRepo)
		templateCatalogueIntegrationSvc = service.NewTemplateCatalogueIntegration(db, jwtAuth, templateCatalogueClient)
		templateRepositorySvc = service.NewTemplateRepository(db, jwtAuth, &ctRepo, &ctRTRepo, &ctATRepo, templateCatalogueClient, auditTrailReader, vcSigner, issuerDID)
		didSrv = didService
		semanticHubSvc = service.NewSemanticHub(db, jwtAuth)
		keyInventorySvc = service.NewKeyInventory(db, jwtAuth)
	}

	// Channel used by background workers and signal handler to notify main to exit.
	errc := make(chan error)

	// Start the PDF lifecycle C2PA subscriber (appends C2PA assertions on state changes).
	// Only start when a real signing URL is configured; without one, the subscriber
	// would attempt signing on every CWE event and log spurious HTTP errors.
	pdfSubClient, err := event.NewNatsSubClient(conf.EventBusTopic(), natsURL)
	if err != nil {
		log.Fatalf(ctx, err, "Could not create PDF generation NATS subscriber")
	}
	defer func(pdfSubClient *event.CloudEventSubClient) {
		err := pdfSubClient.Close()
		if err != nil {
			log.Errorf(ctx, err, "Could not close PDF subscriber")
		}
	}(pdfSubClient)
	pdfSub := &pdfevent.Subscriber{
		DB:        db,
		Artifacts: artifactStore,
		CRepo:     &cweRepo,
		TRepo:     &ctRepo,
		PDFCore:   pdfCoreClient,
		IssuerDID: issuerDID,
		LocalPeer: did,
		VCIssuer:  provenance.NewLocalVCIssuer(vcSigner, issuerDID, statusListPublisher),
	}
	go func() {
		if err := pdfSub.Start(pdfSubClient); err != nil {
			errc <- fmt.Errorf("could not start PDF generation subscriber: %w", err)
		}
	}()

	// Start the auto-deploy subscriber (DCS-FR-CWE-06): once the signing
	// workflow completes (APPLIED_SIGNATURE), it calls the same
	// cwecommand.Deployer the manual POST /contract/deploy endpoint uses.
	deploySubClient, err := event.NewNatsSubClient(conf.EventBusTopic(), natsURL)
	if err != nil {
		log.Fatalf(ctx, err, "Could not create contract-deployment NATS subscriber")
	}
	defer func(deploySubClient *event.CloudEventSubClient) {
		err := deploySubClient.Close()
		if err != nil {
			log.Errorf(ctx, err, "Could not close contract-deployment subscriber")
		}
	}(deploySubClient)
	deploySub := &deployevent.Subscriber{
		Gate: workflowGateCoordinator,
		Deployer: &cwecommand.Deployer{
			DB:             db,
			CRepo:          &cweRepo,
			DeploymentRepo: cweDeploymentRepo,
			TargetRepo:     &cweTargetRepo,
			Target:         contractTargetClient,
			PeerSigs:       &syncRepo,
		},
		LocalPeer: did,
	}
	go func() {
		if err := deploySub.Start(deploySubClient); err != nil {
			errc <- fmt.Errorf("could not start contract-deployment subscriber: %w", err)
		}
	}()

	var (
		authEndpoints                         *genauth.Endpoints
		contractStorageArchiveEndpoints       *contractstoragearchive.Endpoints
		contractWorkflowEngineEndpoints       *contractworkflowengine.Endpoints
		dcsToDcsEndpoints                     *dcstodcs.Endpoints
		pdfGenerationEndpoints                *pdfgeneration.Endpoints
		processAuditAndComplianceEndpoints    *processauditandcompliance.Endpoints
		signatureManagementEndpoints          *signaturemanagement.Endpoints
		templateCatalogueIntegrationEndpoints *templatecatalogueintegration.Endpoints
		templateRepositoryEndpoints           *templaterepository.Endpoints
		didEntpoints                          *didservice.Endpoints
		c2paEndpoints                         *c2paservice.Endpoints
		semanticHubEndpoints                  *semantichubgen.Endpoints
		keyInventoryEndpoints                 *keyinventory.Endpoints
	)
	{
		authEndpoints = genauth.NewEndpoints(authSvc)
		authEndpoints.Use(debug.LogPayloads())
		authEndpoints.Use(log.Endpoint)
		contractStorageArchiveEndpoints = contractstoragearchive.NewEndpoints(contractStorageArchiveSvc)
		contractStorageArchiveEndpoints.Use(debug.LogPayloads())
		contractStorageArchiveEndpoints.Use(log.Endpoint)
		contractWorkflowEngineEndpoints = contractworkflowengine.NewEndpoints(contractWorkflowEngineSvc)
		contractWorkflowEngineEndpoints.Use(debug.LogPayloads())
		contractWorkflowEngineEndpoints.Use(log.Endpoint)
		dcsToDcsEndpoints = dcstodcs.NewEndpoints(dcsToDcsSvc)
		dcsToDcsEndpoints.Use(debug.LogPayloads())
		dcsToDcsEndpoints.Use(log.Endpoint)
		pdfGenerationEndpoints = pdfgeneration.NewEndpoints(pdfGenerationSvc)
		pdfGenerationEndpoints.Use(debug.LogPayloads())
		pdfGenerationEndpoints.Use(log.Endpoint)
		processAuditAndComplianceEndpoints = processauditandcompliance.NewEndpoints(processAuditAndComplianceSvc)
		processAuditAndComplianceEndpoints.Use(auth.AccessMetadataMiddleware)
		processAuditAndComplianceEndpoints.Use(debug.LogPayloads())
		processAuditAndComplianceEndpoints.Use(log.Endpoint)
		signatureManagementEndpoints = signaturemanagement.NewEndpoints(signatureManagementSvc)
		signatureManagementEndpoints.Use(debug.LogPayloads())
		signatureManagementEndpoints.Use(log.Endpoint)
		templateCatalogueIntegrationEndpoints = templatecatalogueintegration.NewEndpoints(templateCatalogueIntegrationSvc)
		templateCatalogueIntegrationEndpoints.Use(debug.LogPayloads())
		templateCatalogueIntegrationEndpoints.Use(log.Endpoint)
		templateRepositoryEndpoints = templaterepository.NewEndpoints(templateRepositorySvc)
		templateRepositoryEndpoints.Use(debug.LogPayloads())
		templateRepositoryEndpoints.Use(log.Endpoint)
		didEntpoints = didservice.NewEndpoints(didSrv)
		didEntpoints.Use(debug.LogPayloads())
		didEntpoints.Use(log.Endpoint)
		c2paEndpoints = c2paservice.NewEndpoints(c2paSvc)
		c2paEndpoints.Use(debug.LogPayloads())
		c2paEndpoints.Use(log.Endpoint)
		semanticHubEndpoints = semantichubgen.NewEndpoints(semanticHubSvc)
		semanticHubEndpoints.Use(debug.LogPayloads())
		semanticHubEndpoints.Use(log.Endpoint)
		keyInventoryEndpoints = keyinventory.NewEndpoints(keyInventorySvc)
		keyInventoryEndpoints.Use(debug.LogPayloads())
		keyInventoryEndpoints.Use(log.Endpoint)
	}

	// SIGINT/SIGTERM stop the service gracefully.
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		errc <- fmt.Errorf("%s", <-c)
	}()

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(ctx)

	// Hand off from the bootstrap server (up since before hsm.Open) to the
	// real one on the same address — safe to do serially since Shutdown
	// blocks until the listener's actually released.
	if err := bootstrapSrv.Shutdown(ctx); err != nil {
		log.Errorf(ctx, err, "failed to shut down bootstrap HTTP server")
	}
	handleHTTPServer(ctx, listenURL, authEndpoints, contractStorageArchiveEndpoints, contractWorkflowEngineEndpoints, dcsToDcsEndpoints, pdfGenerationEndpoints, processAuditAndComplianceEndpoints, signatureManagementEndpoints, templateCatalogueIntegrationEndpoints, templateRepositoryEndpoints, didEntpoints, c2paEndpoints, semanticHubEndpoints, keyInventoryEndpoints, webhookPlatform, statusListSigner, &wg, errc, *dbgF)

	// Wait for signal.
	log.Printf(ctx, "exiting (%v)", <-errc)

	// Send cancellation signal to the goroutines.
	cancel()

	wg.Wait()
	log.Printf(ctx, "exited")
}
