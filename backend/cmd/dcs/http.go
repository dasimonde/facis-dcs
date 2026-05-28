package main

import (
	"context"
	genauth "digital-contracting-service/gen/auth"
	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	externaltargetsystemapi "digital-contracting-service/gen/external_target_system_api"
	authsvr "digital-contracting-service/gen/http/auth/server"
	contractstoragearchivesvr "digital-contracting-service/gen/http/contract_storage_archive/server"
	contractworkflowenginesvr "digital-contracting-service/gen/http/contract_workflow_engine/server"
	dcstodcssvr "digital-contracting-service/gen/http/dcs_to_dcs/server"
	externaltargetsystemapisvr "digital-contracting-service/gen/http/external_target_system_api/server"
	orchestrationwebhookssvr "digital-contracting-service/gen/http/orchestration_webhooks/server"
	processauditandcompliancesvr "digital-contracting-service/gen/http/process_audit_and_compliance/server"
	signaturemanagementsvr "digital-contracting-service/gen/http/signature_management/server"
	templatecatalogueintegrationsvr "digital-contracting-service/gen/http/template_catalogue_integration/server"
	templaterepositorysvr "digital-contracting-service/gen/http/template_repository/server"
	orchestrationwebhooks "digital-contracting-service/gen/orchestration_webhooks"
	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	signaturemanagement "digital-contracting-service/gen/signature_management"
	templatecatalogueintegration "digital-contracting-service/gen/template_catalogue_integration"
	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/service"
	"digital-contracting-service/internal/webhookplatform"
	"net/http"
	"net/url"
	"sync"
	"time"

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
func handleHTTPServer(ctx context.Context, u *url.URL, authEndpoints *genauth.Endpoints, contractStorageArchiveEndpoints *contractstoragearchive.Endpoints, contractWorkflowEngineEndpoints *contractworkflowengine.Endpoints, dcsToDcsEndpoints *dcstodcs.Endpoints, externalTargetSystemAPIEndpoints *externaltargetsystemapi.Endpoints, orchestrationWebhooksEndpoints *orchestrationwebhooks.Endpoints, processAuditAndComplianceEndpoints *processauditandcompliance.Endpoints, signatureManagementEndpoints *signaturemanagement.Endpoints, templateCatalogueIntegrationEndpoints *templatecatalogueintegration.Endpoints, templateRepositoryEndpoints *templaterepository.Endpoints, webhookPlatform *webhookplatform.Platform, wg *sync.WaitGroup, errc chan error, dbg bool) {

	// Provide the transport specific request decoder and response encoder.
	// The goa http package has built-in support for JSON, XML and gob.
	// Other encodings can be used by providing the corresponding functions,
	// see goa.design/implement/encoding.
	var (
		dec = goahttp.RequestDecoder
		enc = goahttp.ResponseEncoder
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

	// Wrap the endpoints with the transport specific layers. The generated
	// server packages contains code generated from the design which maps
	// the service input and output data structures to HTTP requests and
	// responses.
	var (
		authServer                         *authsvr.Server
		contractStorageArchiveServer       *contractstoragearchivesvr.Server
		contractWorkflowEngineServer       *contractworkflowenginesvr.Server
		dcsToDcsServer                     *dcstodcssvr.Server
		externalTargetSystemAPIServer      *externaltargetsystemapisvr.Server
		orchestrationWebhooksServer        *orchestrationwebhookssvr.Server
		processAuditAndComplianceServer    *processauditandcompliancesvr.Server
		signatureManagementServer          *signaturemanagementsvr.Server
		templateCatalogueIntegrationServer *templatecatalogueintegrationsvr.Server
		templateRepositoryServer           *templaterepositorysvr.Server
	)
	{
		eh := errorHandler(ctx)
		ef := errorFormatter
		authServer = authsvr.New(authEndpoints, apiMux, dec, enc, eh, ef)
		contractStorageArchiveServer = contractstoragearchivesvr.New(contractStorageArchiveEndpoints, apiMux, dec, enc, eh, ef)
		contractWorkflowEngineServer = contractworkflowenginesvr.New(contractWorkflowEngineEndpoints, apiMux, dec, enc, eh, ef)
		dcsToDcsServer = dcstodcssvr.New(dcsToDcsEndpoints, apiMux, dec, enc, eh, ef)
		externalTargetSystemAPIServer = externaltargetsystemapisvr.New(externalTargetSystemAPIEndpoints, apiMux, dec, enc, eh, ef)
		orchestrationWebhooksServer = orchestrationwebhookssvr.New(orchestrationWebhooksEndpoints, apiMux, dec, enc, eh, ef)
		processAuditAndComplianceServer = processauditandcompliancesvr.New(processAuditAndComplianceEndpoints, apiMux, dec, enc, eh, ef)
		signatureManagementServer = signaturemanagementsvr.New(signatureManagementEndpoints, apiMux, dec, enc, eh, ef)
		templateCatalogueIntegrationServer = templatecatalogueintegrationsvr.New(templateCatalogueIntegrationEndpoints, apiMux, dec, enc, eh, ef)
		templateRepositoryServer = templaterepositorysvr.New(templateRepositoryEndpoints, apiMux, dec, enc, eh, ef)
	}

	// Configure the mux.
	authsvr.Mount(apiMux, authServer)
	contractstoragearchivesvr.Mount(apiMux, contractStorageArchiveServer)
	contractworkflowenginesvr.Mount(apiMux, contractWorkflowEngineServer)
	dcstodcssvr.Mount(apiMux, dcsToDcsServer)
	externaltargetsystemapisvr.Mount(apiMux, externalTargetSystemAPIServer)
	orchestrationwebhookssvr.Mount(apiMux, orchestrationWebhooksServer)
	processauditandcompliancesvr.Mount(apiMux, processAuditAndComplianceServer)
	signaturemanagementsvr.Mount(apiMux, signatureManagementServer)
	templatecatalogueintegrationsvr.Mount(apiMux, templateCatalogueIntegrationServer)
	templaterepositorysvr.Mount(apiMux, templateRepositoryServer)

	// Mount Swagger UI on /swagger and OpenAPI spec on /openapi3.json.
	mountSwaggerUI(apiMux)

	// Mount frontend static file server (uses base mux, not API mux)
	mountFrontend(mux)

	// Outer mux: routes /orce/* to the webhook platform, everything else to Goa.
	outerMux := http.NewServeMux()
	outerMux.Handle("/orce/", http.StripPrefix("/orce", webhookPlatform))
	outerMux.Handle("/metrics", promhttp.Handler())
	outerMux.Handle("/", mux)

	var handler http.Handler = outerMux
	handler = service.RequestContextMiddleware(handler)
	handler = middleware.InjectIP(handler)
	handler = metricsMiddleware(handler)
	if dbg {
		// Log query and response bodies if debug logs are enabled.
		handler = debug.HTTP()(handler)
	}
	handler = log.HTTP(ctx)(handler)

	// Start HTTP server using default configuration, change the code to
	// configure the server as required by your service.
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
	for _, m := range externalTargetSystemAPIServer.Mounts {
		log.Printf(ctx, "HTTP %q mounted on %s %s", m.Method, m.Verb, m.Pattern)
	}
	for _, m := range orchestrationWebhooksServer.Mounts {
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

// errorFormatter maps named ServiceErrors ("unauthorized", "forbidden") to the
// correct HTTP status codes. All other errors fall through to the default Goa
// heuristic.
func errorFormatter(ctx context.Context, err error) goahttp.Statuser {
	resp := goahttp.NewErrorResponse(ctx, err)

	var gerr *goa.ServiceError
	if errors.As(err, &gerr) {
		switch gerr.Name {
		case "unauthorized":
			return &errorResponse{ErrorResponse: resp.(*goahttp.ErrorResponse), statusCode: http.StatusUnauthorized}
		case "forbidden":
			return &errorResponse{ErrorResponse: resp.(*goahttp.ErrorResponse), statusCode: http.StatusForbidden}
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
