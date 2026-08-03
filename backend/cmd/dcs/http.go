package main

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	didservice "digital-contracting-service/gen/did_service"

	genauth "digital-contracting-service/gen/auth"
	c2paservice "digital-contracting-service/gen/c2_pa_service"
	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	authsvr "digital-contracting-service/gen/http/auth/server"
	c2pasvr "digital-contracting-service/gen/http/c2_pa_service/server"
	contractstoragearchivesvr "digital-contracting-service/gen/http/contract_storage_archive/server"
	contractworkflowenginesvr "digital-contracting-service/gen/http/contract_workflow_engine/server"
	dcstodcssvr "digital-contracting-service/gen/http/dcs_to_dcs/server"
	didsvr "digital-contracting-service/gen/http/did_service/server"
	keyinventorysvr "digital-contracting-service/gen/http/key_inventory/server"
	pdfgenerationsvr "digital-contracting-service/gen/http/pdf_generation/server"
	processauditandcompliancesvr "digital-contracting-service/gen/http/process_audit_and_compliance/server"
	semantichubsvr "digital-contracting-service/gen/http/semantic_hub/server"
	signaturemanagementsvr "digital-contracting-service/gen/http/signature_management/server"
	templatecatalogueintegrationsvr "digital-contracting-service/gen/http/template_catalogue_integration/server"
	templaterepositorysvr "digital-contracting-service/gen/http/template_repository/server"
	keyinventory "digital-contracting-service/gen/key_inventory"
	pdfgeneration "digital-contracting-service/gen/pdf_generation"
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	semantichubgen "digital-contracting-service/gen/semantic_hub"
	signaturemanagement "digital-contracting-service/gen/signature_management"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
	"digital-contracting-service/internal/service"
	"digital-contracting-service/internal/webhookplatform"

	"errors"

	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"goa.design/clue/debug"
	"goa.design/clue/log"
	goahttp "goa.design/goa/v3/http"
	goa "goa.design/goa/v3/pkg"
)

type formRequestDecoder struct {
	r *http.Request
}

type rawBytesEncoder struct {
	fallback goahttp.Encoder
	w        http.ResponseWriter
}

func (e rawBytesEncoder) Encode(value any) error {
	if data, ok := value.([]byte); ok {
		_, err := e.w.Write(data)
		return err
	}
	return e.fallback.Encode(value)
}

func responseEncoder(ctx context.Context, w http.ResponseWriter) goahttp.Encoder {
	return rawBytesEncoder{fallback: goahttp.ResponseEncoder(ctx, w), w: w}
}

func (d *formRequestDecoder) Decode(v any) error {
	if err := d.r.ParseForm(); err != nil {
		return fmt.Errorf("parse form body: %w", err)
	}

	m := make(map[string]any, len(d.r.PostForm))
	for key, values := range d.r.PostForm {
		if len(values) == 0 {
			continue
		}
		m[key] = values[0]
	}

	raw, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal form payload: %w", err)
	}

	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("decode form payload: %w", err)
	}

	return nil
}

func requestDecoderWithForm(r *http.Request) goahttp.Decoder {
	if r != nil {
		contentType := r.Header.Get("Content-Type")
		mediaType, _, err := mime.ParseMediaType(contentType)
		if (err == nil && mediaType == "application/x-www-form-urlencoded") || r.ContentLength == 0 {
			return &formRequestDecoder{r: r}
		}
	}

	return goahttp.RequestDecoder(r)
}

var (
	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Duration of HTTP requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
)

// handleHTTPServer starts configures and starts a HTTP server on the given
// URL. It shuts down the server if any error is received in the error channel.
func handleHTTPServer(ctx context.Context, u *url.URL, authEndpoints *genauth.Endpoints,
	contractStorageArchiveEndpoints *contractstoragearchive.Endpoints, contractWorkflowEngineEndpoints *contractworkflowengine.Endpoints,
	dcsToDcsEndpoints *dcstodcs.Endpoints, pdfGenerationEndpoints *pdfgeneration.Endpoints, processAuditAndComplianceEndpoints *processauditandcompliance.Endpoints,
	signatureManagementEndpoints *signaturemanagement.Endpoints, templateCatalogueIntegrationEndpoints *templatecatalogueintegration.Endpoints,
	templateRepositoryEndpoints *templaterepository.Endpoints, didEnpoints *didservice.Endpoints, c2paEndpoints *c2paservice.Endpoints, semanticHubEndpoints *semantichubgen.Endpoints, keyInventoryEndpoints *keyinventory.Endpoints, webhookPlatform *webhookplatform.Platform, statusList *provenance.StatusListSigner, wg *sync.WaitGroup,
	errc chan error, dbg bool) {

	var (
		dec = requestDecoderWithForm
		enc = responseEncoder
	)

	// Build the service HTTP request multiplexer and mount debug and profiler
	// endpoints in debug mode.
	var mux goahttp.Muxer
	{
		mux = goahttp.NewMuxer()
		if dbg {
			debug.MountPprofHandlers(debug.Adapt(mux))
			debug.MountDebugLogEnabler(debug.Adapt(mux))
		}
	}

	// Apply API path prefix if configured
	apiPrefix := getAPIPathPrefix()
	apiMux := newPrefixedMuxer(mux, apiPrefix)

	var (
		authServer                         *authsvr.Server
		contractStorageArchiveServer       *contractstoragearchivesvr.Server
		contractWorkflowEngineServer       *contractworkflowenginesvr.Server
		dcsToDcsServer                     *dcstodcssvr.Server
		pdfGenerationServer                *pdfgenerationsvr.Server
		processAuditAndComplianceServer    *processauditandcompliancesvr.Server
		signatureManagementServer          *signaturemanagementsvr.Server
		templateCatalogueIntegrationServer *templatecatalogueintegrationsvr.Server
		templateRepositoryServer           *templaterepositorysvr.Server
		didServer                          *didsvr.Server
		c2paServer                         *c2pasvr.Server
		semanticHubServer                  *semantichubsvr.Server
		keyInventoryServer                 *keyinventorysvr.Server
	)
	{
		eh := errorHandler(ctx)
		ef := errorFormatter
		authServer = authsvr.New(authEndpoints, apiMux, dec, enc, eh, ef)
		contractStorageArchiveServer = contractstoragearchivesvr.New(contractStorageArchiveEndpoints, apiMux, dec, enc, eh, ef)
		contractWorkflowEngineServer = contractworkflowenginesvr.New(contractWorkflowEngineEndpoints, apiMux, dec, enc, eh, ef)
		dcsToDcsServer = dcstodcssvr.New(dcsToDcsEndpoints, apiMux, dec, enc, eh, ef)
		pdfGenerationServer = pdfgenerationsvr.New(pdfGenerationEndpoints, apiMux, dec, enc, eh, ef)
		processAuditAndComplianceServer = processauditandcompliancesvr.New(processAuditAndComplianceEndpoints, apiMux, dec, enc, eh, ef)
		signatureManagementServer = signaturemanagementsvr.New(signatureManagementEndpoints, apiMux, dec, enc, eh, ef)
		templateCatalogueIntegrationServer = templatecatalogueintegrationsvr.New(templateCatalogueIntegrationEndpoints, apiMux, dec, enc, eh, ef)
		templateRepositoryServer = templaterepositorysvr.New(templateRepositoryEndpoints, apiMux, dec, enc, eh, ef)
		didServer = didsvr.New(didEnpoints, apiMux, dec, enc, eh, ef)
		c2paServer = c2pasvr.New(c2paEndpoints, apiMux, dec, enc, eh, ef)
		semanticHubServer = semantichubsvr.New(semanticHubEndpoints, apiMux, dec, enc, eh, ef)
		keyInventoryServer = keyinventorysvr.New(keyInventoryEndpoints, apiMux, dec, enc, eh, ef)
	}

	// did.json is served at the origin root (did:web well-known path), outside
	// the API prefix.
	didsvr.Mount(mux, didServer)
	c2pasvr.Mount(apiMux, c2paServer)
	authsvr.Mount(apiMux, authServer)
	contractstoragearchivesvr.Mount(apiMux, contractStorageArchiveServer)
	contractworkflowenginesvr.Mount(apiMux, contractWorkflowEngineServer)
	dcstodcssvr.Mount(apiMux, dcsToDcsServer)
	pdfgenerationsvr.Mount(apiMux, pdfGenerationServer)
	processauditandcompliancesvr.Mount(apiMux, processAuditAndComplianceServer)
	signaturemanagementsvr.Mount(apiMux, signatureManagementServer)
	templatecatalogueintegrationsvr.Mount(apiMux, templateCatalogueIntegrationServer)
	templaterepositorysvr.Mount(apiMux, templateRepositoryServer)
	semantichubsvr.Mount(apiMux, semanticHubServer)
	keyinventorysvr.Mount(apiMux, keyInventoryServer)

	// Mount Swagger UI on /swagger and OpenAPI spec on /openapi3.json.
	mountSwaggerUI(apiMux)

	// Mount frontend static file server (uses base mux, not API mux)
	mountFrontend(mux)

	// Outer mux: routes /orce/* to the webhook platform, everything else to Goa.
	outerMux := http.NewServeMux()
	outerMux.Handle("/orce/", http.StripPrefix("/orce", webhookPlatform))
	// The status list for the credentials this deployment issues (ADR-34). It
	// sits here rather than in the generated API surface for two reasons: it is
	// served at the origin root, the way did.json is, so a verifier holding only
	// a credential can reach it without knowing this deployment's API prefix; and
	// its media type is the routing signal a verifier uses, which Goa's response
	// encoder would renegotiate to application/json.
	outerMux.Handle(provenance.StatusListPath, provenance.StatusListHandler(statusList))
	outerMux.Handle("/metrics", promhttp.Handler())
	mountReadinessEndpoint(outerMux)
	outerMux.Handle("/", mux)

	var handler http.Handler = outerMux
	handler = middleware.RateLimitAuthenticated(conf.APIRateLimitPerMinute(), handler)
	handler = reportContentTypeMiddleware(handler)
	handler = service.RequestContextMiddleware(handler)
	handler = middleware.InjectIP(handler)
	handler = metricsMiddleware(handler)
	if dbg {
		// Log query and response bodies if debug logs are enabled.
		handler = debug.HTTP()(handler)
	}
	handler = log.HTTP(ctx)(handler)

	srv := &http.Server{Addr: u.Host, Handler: handler, ReadHeaderTimeout: time.Second * 60}
	for _, m := range authServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range contractStorageArchiveServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range contractWorkflowEngineServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range dcsToDcsServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range pdfGenerationServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range processAuditAndComplianceServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range signatureManagementServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range templateCatalogueIntegrationServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range templateRepositoryServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range semanticHubServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range didServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range c2paServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range keyInventoryServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}

	(*wg).Add(1)
	go func() {
		defer (*wg).Done()

		// Start HTTP server in a separate goroutine.
		go func() {
			log.Printf(ctx, "HTTP server listening on %q", u.Host)
			errc <- srv.ListenAndServe()
		}()

		<-ctx.Done()
		log.Printf(ctx, "shutting down HTTP server at %q", u.Host)

		// Shutdown gracefully with a 30s timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := srv.Shutdown(ctx)
		if err != nil {
			log.Printf(ctx, "failed to shutdown: %v", err)
		}
	}()
}

func reportContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pac/report") && r.Method == http.MethodGet {
			switch strings.ToLower(r.URL.Query().Get("format")) {
			case "csv":
				w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			case "pdf":
				w.Header().Set("Content-Type", "application/pdf")
			default:
				w.Header().Set("Content-Type", "application/json")
			}
		}
		next.ServeHTTP(w, r)
	})
}

// errorHandler returns a function that writes and logs the given error.
// The function also writes and logs the error unique ID so that it's possible
// to correlate.
func errorHandler(logCtx context.Context) func(context.Context, http.ResponseWriter, error) {
	return func(ctx context.Context, w http.ResponseWriter, err error) {
		log.Printf(logCtx, "ERROR: %s", err.Error())
	}
}

// errorResponse wraps a goahttp.ErrorResponse with a custom status code.
type errorResponse struct {
	*goahttp.ErrorResponse
	statusCode int
}

func (e *errorResponse) StatusCode() int { return e.statusCode }

// bundleExportRefusedResponse preserves the structural-integrity findings of a
// *pdfgeneration.BundleExportRefusedError in the HTTP body. The default Goa
// error heuristic (goahttp.NewErrorResponse) only understands *goa.ServiceError
// and would otherwise collapse the refusal into the generic
// {name,id,message,temporary,timeout,fault} hull, dropping the findings array
// clients rely on. The explicit lowercase JSON tags match the
// generated ExportContractBundleRefusedResponseBody so the wire format is
// identical to the design's "refused" response body.
type bundleExportRefusedResponse struct {
	Name     string   `json:"name"`
	Message  string   `json:"message"`
	Findings []string `json:"findings"`
}

func (e *bundleExportRefusedResponse) StatusCode() int { return http.StatusUnprocessableEntity }

type workflowGateBlockedResponse struct {
	Name      string   `json:"name"`
	Message   string   `json:"message"`
	GateRunID string   `json:"gate_run_id,omitempty"`
	Status    string   `json:"status"`
	Findings  []string `json:"findings,omitempty"`
}

func (e *workflowGateBlockedResponse) StatusCode() int {
	if e.Status == "BLOCKED" {
		return http.StatusUnprocessableEntity
	}
	return http.StatusConflict
}

// errorFormatter maps named ServiceErrors to the correct HTTP status codes.
// All other errors fall through to the default Goa heuristic.
func errorFormatter(ctx context.Context, err error) goahttp.Statuser {
	var gateBlocked *workflowgate.BlockedError
	if errors.As(err, &gateBlocked) {
		response := &workflowGateBlockedResponse{
			Name: "workflow_gate_blocked", Message: gateBlocked.Error(),
			GateRunID: gateBlocked.RunID, Status: gateBlocked.Status,
		}
		var localBlocked *workflowgate.LocalEvaluationBlockedError
		if errors.As(err, &localBlocked) {
			response.Findings = localBlocked.Reasons()
		}
		return response
	}

	// A bundle-export refusal is its own error type (not a *goa.ServiceError),
	// so it must be handled before the generic heuristic that would discard its
	// findings. This covers both ExportContractBundle and ExportTemplateBundle,
	// which share the type.
	var refused *pdfgeneration.BundleExportRefusedError
	if errors.As(err, &refused) {
		findings := refused.Findings
		if findings == nil {
			findings = []string{}
		}
		return &bundleExportRefusedResponse{
			Name:     refused.Name,
			Message:  refused.Message,
			Findings: findings,
		}
	}

	resp := goahttp.NewErrorResponse(ctx, err)

	var gerr *goa.ServiceError
	if errors.As(err, &gerr) {
		switch gerr.Name {
		case "bad_request":
			return &errorResponse{ErrorResponse: resp.(*goahttp.ErrorResponse), statusCode: http.StatusBadRequest}
		case "unauthorized":
			return &errorResponse{ErrorResponse: resp.(*goahttp.ErrorResponse), statusCode: http.StatusUnauthorized}
		case "forbidden":
			return &errorResponse{ErrorResponse: resp.(*goahttp.ErrorResponse), statusCode: http.StatusForbidden}
		case "not_found":
			return &errorResponse{ErrorResponse: resp.(*goahttp.ErrorResponse), statusCode: http.StatusNotFound}
		case "service_unavailable":
			return &errorResponse{ErrorResponse: resp.(*goahttp.ErrorResponse), statusCode: http.StatusServiceUnavailable}
		}
	}

	return resp
}

func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(wrapped, r)
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(wrapped.status)

		httpRequestDuration.WithLabelValues(r.Method, r.URL.Path, status).Observe(duration)
		httpRequestsTotal.WithLabelValues(r.Method, r.URL.Path, status).Inc()
	})
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}
