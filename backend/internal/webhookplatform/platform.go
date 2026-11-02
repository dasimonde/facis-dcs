package webhookplatform

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"
)

// CallbackHandler is called when ORCE sends a callback after processing a webhook.
type CallbackHandler func(ctx context.Context, pending PendingCallback, status string, result json.RawMessage)

// Caller is the validated identity behind a Bearer token.
type Caller struct {
	HolderDID string
	// ParticipantDID is the caller's organization. It owns the subscriptions
	// the caller creates.
	ParticipantDID string
	Roles          []string
}

// TokenValidator validates a Bearer token and returns the caller.
type TokenValidator func(ctx context.Context, token string) (Caller, error)

// managementRoles may create, list and delete webhook subscriptions. A
// subscription connects a third-party system to DCS events, which ADR-29
// assigns to the Integration Manager and the System Administrator. The
// machine role Sys. Integration Manager lets an integration (ORCE) manage its
// subscriptions with its own credential.
var managementRoles = []string{
	userrole.IntegrationManager.String(),
	userrole.SystemAdministrator.String(),
	userrole.SystemIntegrationManager.String(),
}

// Platform is the webhook subscription HTTP handler.
// Mount it on any path prefix with http.StripPrefix.
type Platform struct {
	subs       Subscriptions
	store      Deliveries
	dispatcher *Dispatcher
	validate   TokenValidator
	onCallback CallbackHandler
	mux        *http.ServeMux
}

// New creates a ready-to-use Platform.
// validate:   JWT validator — use middleware.HydraJWTValidator.ValidateToken
// onCallback: called when ORCE POSTs to /callbacks (may be nil)
func New(subs Subscriptions, store Deliveries, dispatcher *Dispatcher, validate TokenValidator, onCallback CallbackHandler) *Platform {
	p := &Platform{
		subs:       subs,
		store:      store,
		dispatcher: dispatcher,
		validate:   validate,
		onCallback: onCallback,
		mux:        http.NewServeMux(),
	}
	p.routes()
	return p
}

func (p *Platform) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mux.ServeHTTP(w, r)
}

// routes wires all endpoints.
// GET  /health     — no auth (liveness probe)
// GET  /events     — authenticated
// GET  /webhooks   — management role
// POST /webhooks   — management role
// DELETE /webhooks/{id} — management role
// POST /callbacks  — management role
func (p *Platform) routes() {
	p.mux.HandleFunc("GET /health", p.health)
	p.mux.HandleFunc("GET /events", p.auth(p.listEvents))
	p.mux.HandleFunc("GET /webhooks", p.auth(p.manage(p.listWebhooks)))
	p.mux.HandleFunc("POST /webhooks", p.auth(p.manage(p.subscribe)))
	p.mux.HandleFunc("DELETE /webhooks/{id}", p.auth(p.manage(p.unsubscribe)))
	p.mux.HandleFunc("POST /callbacks", p.auth(p.manage(p.callback)))
	p.mux.HandleFunc("GET /deliveries", p.auth(p.manage(p.listDeliveries)))
}

// ── Middleware ────────────────────────────────────────────────────────────────

// auth wraps a handler with JWT Bearer token validation.
func (p *Platform) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			jsonError(w, http.StatusUnauthorized, "missing Bearer token")
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")

		caller, err := p.validate(r.Context(), token)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyCaller{}, caller)
		next(w, r.WithContext(ctx))
	}
}

// manage admits only callers with a management role and an organization to
// scope their subscriptions to.
func (p *Platform) manage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller := callerFromCtx(r.Context())
		isManager := slices.ContainsFunc(caller.Roles, func(role string) bool {
			return slices.Contains(managementRoles, role)
		})
		if !isManager || caller.ParticipantDID == "" {
			jsonError(w, http.StatusForbidden, "a webhook subscription management role is required")
			return
		}
		next(w, r)
	}
}

type ctxKeyCaller struct{}

func callerFromCtx(ctx context.Context) Caller {
	v, _ := ctx.Value(ctxKeyCaller{}).(Caller)
	return v
}

func holderDIDFromCtx(ctx context.Context) string {
	return callerFromCtx(ctx).HolderDID
}

// subscriptionView is the response shape of a subscription. It has no secret;
// only POST /webhooks adds it, once.
type subscriptionView struct {
	ID          string    `json:"id"`
	Event       string    `json:"event"`
	CallbackURL string    `json:"callback_url"`
	CreatedAt   time.Time `json:"created_at"`
}

func viewOf(sub Subscription) subscriptionView {
	return subscriptionView{ID: sub.ID, Event: sub.Event, CallbackURL: sub.CallbackURL, CreatedAt: sub.CreatedAt}
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func (p *Platform) health(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "ok"})
}

// GET /events
func (p *Platform) listEvents(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, KnownEvents)
}

// GET /webhooks
func (p *Platform) listWebhooks(w http.ResponseWriter, r *http.Request) {
	subs, err := p.subs.ListByOwner(r.Context(), callerFromCtx(r.Context()).ParticipantDID)
	if err != nil {
		log.Printf("webhookplatform: %v", err)
		jsonError(w, http.StatusInternalServerError, "could not list subscriptions")
		return
	}
	views := make([]subscriptionView, 0, len(subs))
	for _, sub := range subs {
		views = append(views, viewOf(sub))
	}
	jsonOK(w, views)
}

// GET /deliveries — recent notification outcomes (status code / error /
// acknowledged) of the caller organization's subscriptions, the monitoring
// surface for DCS-FR-TR-22 subscribers.
func (p *Platform) listDeliveries(w http.ResponseWriter, r *http.Request) {
	deliveries, err := p.store.ListDeliveries(r.Context(), callerFromCtx(r.Context()).ParticipantDID)
	if err != nil {
		log.Printf("webhookplatform: %v", err)
		jsonError(w, http.StatusInternalServerError, "could not list deliveries")
		return
	}
	jsonOK(w, deliveries)
}

// POST /webhooks
func (p *Platform) subscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Event       string          `json:"event"`
		CallbackURL string          `json:"callback_url"`
		Secret      json.RawMessage `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Event == "" || req.CallbackURL == "" {
		jsonError(w, http.StatusBadRequest, "event and callback_url are required")
		return
	}
	if err := ValidateCallbackURL(req.CallbackURL); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error()+"; plain http only for a host the deployment names")
		return
	}
	if !isKnownEvent(req.Event) {
		jsonError(w, http.StatusBadRequest, "unknown event: "+req.Event)
		return
	}
	// A client that sends its own secret expects deliveries signed with it.
	// DCS does not use it, so the request fails and the client notices.
	if req.Secret != nil {
		jsonError(w, http.StatusBadRequest, "do not send a secret: DCS generates it and returns it in the 201 response")
		return
	}
	secret, err := NewSecret()
	if err != nil {
		log.Printf("webhookplatform: %v", err)
		jsonError(w, http.StatusInternalServerError, "could not create subscription")
		return
	}

	caller := callerFromCtx(r.Context())
	sub, err := p.subs.Add(r.Context(), Subscription{
		Event:       req.Event,
		CallbackURL: req.CallbackURL,
		Secret:      secret,
		OwnerDID:    caller.ParticipantDID,
		CreatedBy:   caller.HolderDID,
	})
	if err != nil {
		log.Printf("webhookplatform: %v", err)
		jsonError(w, http.StatusInternalServerError, "could not store subscription")
		return
	}
	log.Printf("webhookplatform: new subscription id=%s event=%s url=%s caller=%s owner=%s",
		sub.ID, sub.Event, sub.CallbackURL, caller.HolderDID, caller.ParticipantDID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	// The only response that carries the secret: DCS does not show it again.
	jsonOK(w, struct {
		subscriptionView
		Secret string `json:"secret"`
	}{viewOf(sub), secret})
}

// DELETE /webhooks/{id}
//
// A subscription of another organization gives 404 like a missing one, so the
// response does not show which ids exist for other organizations.
func (p *Platform) unsubscribe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	deleted, err := p.subs.DeleteOwned(r.Context(), id, callerFromCtx(r.Context()).ParticipantDID)
	if err != nil {
		log.Printf("webhookplatform: %v", err)
		jsonError(w, http.StatusInternalServerError, "could not delete subscription")
		return
	}
	if !deleted {
		jsonError(w, http.StatusNotFound, "subscription not found")
		return
	}
	log.Printf("webhookplatform: deleted subscription id=%s caller=%s",
		id, holderDIDFromCtx(r.Context()))
	w.WriteHeader(http.StatusNoContent)
}

// POST /callbacks
func (p *Platform) callback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID string          `json:"event_id"`
		Status  string          `json:"status"`
		Result  json.RawMessage `json:"result,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.EventID == "" || req.Status == "" {
		jsonError(w, http.StatusBadRequest, "event_id and status are required")
		return
	}

	// Only an organization that received the event acknowledges it, and
	// only its own deliveries.
	pending, ok, err := p.store.ResolvePending(r.Context(), req.EventID, callerFromCtx(r.Context()).ParticipantDID)
	if err != nil {
		log.Printf("webhookplatform: %v", err)
		jsonError(w, http.StatusInternalServerError, "could not resolve the callback")
		return
	}
	if !ok {
		jsonError(w, http.StatusNotFound, "unknown event_id")
		return
	}

	log.Printf("webhookplatform: callback received event_id=%s event=%s did=%s status=%s caller=%s",
		req.EventID, pending.Event, pending.DID, req.Status, holderDIDFromCtx(r.Context()))

	if p.onCallback != nil {
		go p.onCallback(context.WithoutCancel(r.Context()), pending, req.Status, req.Result)
	}

	jsonOK(w, map[string]string{"status": "accepted"})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("webhookplatform: failed to encode JSON response: %v", err)
	}
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": msg}); err != nil {
		log.Printf("webhookplatform: failed to encode JSON error response: %v", err)
	}
}

func isKnownEvent(name string) bool {
	for _, e := range KnownEvents {
		if e.Name == name {
			return true
		}
	}
	return false
}
