package main

import (
	"context"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

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
	infosvr "digital-contracting-service/gen/http/info/server"
	keyinventorysvr "digital-contracting-service/gen/http/key_inventory/server"
	pdfgenerationsvr "digital-contracting-service/gen/http/pdf_generation/server"
	processauditandcompliancesvr "digital-contracting-service/gen/http/process_audit_and_compliance/server"
	semantichubsvr "digital-contracting-service/gen/http/semantic_hub/server"
	signaturemanagementsvr "digital-contracting-service/gen/http/signature_management/server"
	templatecatalogueintegrationsvr "digital-contracting-service/gen/http/template_catalogue_integration/server"
	templaterepositorysvr "digital-contracting-service/gen/http/template_repository/server"
	geninfo "digital-contracting-service/gen/info"
	keyinventory "digital-contracting-service/gen/key_inventory"
	pdfgeneration "digital-contracting-service/gen/pdf_generation"
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	semantichubgen "digital-contracting-service/gen/semantic_hub"
	signaturemanagement "digital-contracting-service/gen/signature_management"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/pathutil"
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
	if ctx.Value(goa.ServiceKey) == processauditandcompliance.ServiceName && ctx.Value(goa.MethodKey) == "audit_report" {
		// The format query selects report bytes; errors always use JSON.
		return rawBytesEncoder{fallback: json.NewEncoder(w), w: w}
	}
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
	templateRepositoryEndpoints *templaterepository.Endpoints, didEnpoints *didservice.Endpoints, c2paEndpoints *c2paservice.Endpoints, semanticHubEndpoints *semantichubgen.Endpoints, keyInventoryEndpoints *keyinventory.Endpoints, infoEndpoints *geninfo.Endpoints, webhookPlatform *webhookplatform.Platform, statusList *provenance.StatusListSigner, db *sqlx.DB, wg *sync.WaitGroup,
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
		infoServer                         *infosvr.Server
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
		infoServer = infosvr.New(infoEndpoints, apiMux, dec, enc, eh, ef)
	}

	// did.json is served at the origin root (did:web well-known path), outside
	// the API prefix.
	didsvr.Mount(mux, didServer)
	infosvr.Mount(mux, infoServer)
	infosvr.Mount(apiMux, infoServer)
	// The C2PA manifest store is the public sibling of did.json (ADR-4,
	// DCS-OR-C2PA-008): an external verifier resolves a contract's provenance
	// from the manifest URL alone, so the route has to answer at the origin
	// root, not only under the API prefix. Root-mounted like the DID service;
	// the prefixed mount below stays for API clients.
	c2pasvr.Mount(mux, c2paServer)
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

	outerMux := newOuterMux(webhookPlatform, provenance.StatusListHandler(statusList), mux)

	handler := recordRoute(outerMux)
	handler = middleware.ResponseHeaders(handler)
	handler = reportContentTypeMiddleware(handler)
	handler = evidenceBodyLimit(handler)
	handler = service.RequestContextMiddleware(handler)
	handler = middleware.InjectIP(handler)
	handler = metricsMiddleware(handler)
	handler = httpsMiddleware(os.Getenv("DCS_PUBLIC_BASE_URL"), handler)
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
	go func() { // #nosec G118 -- server lifecycle goroutine; it outlives the request contexts by design
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

// newOuterMux routes /orce/* to the webhook platform, the status list and
// /readyz to their handlers, and everything else to Goa. It does not serve
// /metrics: that is on the metrics listener (see serveMetrics). The webhook
// platform is also under the API path (DCS_API_PATH), where the ingress routes
// the calls of the Webhooks view (DCS-NFR-SQ-08).
func newOuterMux(webhookPlatform, statusList, goaMux http.Handler) *http.ServeMux {
	outerMux := http.NewServeMux()
	outerMux.Handle("/orce/", http.StripPrefix("/orce", webhookPlatform))
	if apiRoot := strings.TrimSuffix(pathutil.NormalizePath(os.Getenv("DCS_API_PATH"), "", false), "/"); apiRoot != "" {
		outerMux.Handle(apiRoot+"/orce/", http.StripPrefix(apiRoot+"/orce", webhookPlatform))
	}
	// The status list for the credentials this deployment issues (ADR-34). It
	// sits here rather than in the generated API surface for two reasons: it is
	// served at the origin root, the way did.json is, so a verifier holding only
	// a credential can reach it without knowing this deployment's API prefix; and
	// its media type is the routing signal a verifier uses, which Goa's response
	// encoder would renegotiate to application/json.
	outerMux.Handle(provenance.StatusListPath, statusList)
	mountReadinessEndpoint(outerMux)
	outerMux.Handle("/", goaMux)
	return outerMux
}

const defaultMetricsPort = "9464"

func newMetricsHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	return mux
}

// serveMetrics serves Prometheus metrics on DCS_METRICS_PORT (default 9464).
// The chart exposes that port only through a ClusterIP Service, so /metrics
// stays cluster-internal. A listener failure is logged and does not stop the
// service: metrics are not needed to serve contracts.
func serveMetrics(ctx context.Context, wg *sync.WaitGroup) {
	port := os.Getenv("DCS_METRICS_PORT")
	if port == "" {
		port = defaultMetricsPort
	}
	srv := &http.Server{Addr: net.JoinHostPort("", port), Handler: newMetricsHandler(), ReadHeaderTimeout: time.Second * 60}

	wg.Add(1)
	go func() { // #nosec G118 -- server lifecycle goroutine; it outlives the request contexts by design
		defer wg.Done()
		go func() {
			log.Printf(ctx, "metrics server listening on %q", srv.Addr)
			if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
				log.Errorf(ctx, err, "metrics server stopped; /metrics is unavailable")
			}
		}()
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf(ctx, "failed to shutdown metrics server: %v", err)
		}
	}()
}

// hstsHeader tells the browser to use HTTPS only for this host for one year.
const hstsHeader = "max-age=31536000"

// httpsMiddleware delivers browser and API access over HTTPS behind a
// TLS-terminating proxy (DCS-IR-CI-03). It acts only when publicBaseURL is an
// https URL. A request that the proxy received over plain HTTP
// (X-Forwarded-Proto: http) gets a permanent redirect to the same path on the
// public HTTPS origin. A response to a request received over HTTPS carries
// HSTS. A request without X-Forwarded-Proto (probes, in-cluster calls) passes
// unchanged.
func httpsMiddleware(publicBaseURL string, next http.Handler) http.Handler {
	public, err := url.Parse(strings.TrimSpace(publicBaseURL))
	if err != nil || public.Scheme != "https" || public.Host == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A proxy that appends adds its hop on the right; the entries to its
		// left are sent by the client.
		var proto string
		if values := r.Header.Values("X-Forwarded-Proto"); len(values) > 0 {
			proto = values[len(values)-1]
			if i := strings.LastIndex(proto, ","); i >= 0 {
				proto = proto[i+1:]
			}
		}
		switch strings.ToLower(strings.TrimSpace(proto)) {
		case "http":
			// 308 keeps the method and the body of a POST.
			http.Redirect(w, r, "https://"+public.Host+r.URL.RequestURI(), http.StatusPermanentRedirect) // #nosec G710 -- the redirect host is the configured public host, not taken from the request
			return
		case "https":
			w.Header().Set("Strict-Transport-Security", hstsHeader)
		}
		next.ServeHTTP(w, r)
	})
}

func reportContentTypeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/pac/report") && r.Method == http.MethodGet {
			contentType := "application/json"
			switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))) {
			case "csv":
				contentType = "text/csv; charset=utf-8"
			case "pdf":
				contentType = "application/pdf"
			}
			w = &reportResponseWriter{ResponseWriter: w, contentType: contentType}
		}
		next.ServeHTTP(w, r)
	})
}

type reportResponseWriter struct {
	http.ResponseWriter
	contentType string
	written     bool
}

func (w *reportResponseWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	contentType := "application/json"
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		contentType = w.contentType
	}
	w.Header().Set("Content-Type", contentType)
	w.ResponseWriter.WriteHeader(status)
	w.written = status >= http.StatusOK
}

func (w *reportResponseWriter) Write(data []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *reportResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

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
		case "conflict":
			return &errorResponse{ErrorResponse: resp.(*goahttp.ErrorResponse), statusCode: http.StatusConflict}
		case "too_many_requests":
			return &errorResponse{ErrorResponse: resp.(*goahttp.ErrorResponse), statusCode: http.StatusTooManyRequests}
		}
	}

	return resp
}

// metricsMiddleware labels the request metrics with values from bounded
// sets only (DCS-NFR-PER-01): the route pattern that served the request, or
// "unmatched", and a known HTTP method, or "OTHER". So requests to unknown
// paths or with invented methods add no new series.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: 200}
		route := new(string)
		next.ServeHTTP(wrapped, r.WithContext(context.WithValue(r.Context(), routeKey{}, route)))
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(wrapped.status)
		method := metricsMethod(r.Method)
		path := *route
		if path == "" {
			path = "unmatched"
		}

		httpRequestDuration.WithLabelValues(method, path, status).Observe(duration)
		httpRequestsTotal.WithLabelValues(method, path, status).Inc()
	})
}

type routeKey struct{}

// recordRoute wraps the router. The routers set r.Pattern on the request they
// get, so after the request it holds the pattern of the route that served it:
// a Goa route ("/contract/{did}"), an outer route ("/orce/"), or "/" when no
// route below the outer router matched.
func recordRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		route, ok := r.Context().Value(routeKey{}).(*string)
		if !ok {
			return
		}
		pattern := r.Pattern
		if i := strings.Index(pattern, "/"); i >= 0 {
			pattern = pattern[i:]
		}
		*route = pattern
	})
}

func metricsMethod(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	}
	return "OTHER"
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController flush through to the connection, which
// the server-sent event stream needs.
func (rw *responseWriter) Unwrap() http.ResponseWriter { return rw.ResponseWriter }
