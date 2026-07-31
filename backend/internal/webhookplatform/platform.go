package webhookplatform

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// CallbackHandler is called when ORCE sends a callback after processing a webhook.
type CallbackHandler func(ctx context.Context, pending PendingCallback, status string, result json.RawMessage)

// TokenValidator validates a Bearer token and returns the caller's holderDID.
type TokenValidator func(ctx context.Context, token string) (holderDID string, err error)

// Platform is the webhook subscription HTTP handler.
// Mount it on any path prefix with http.StripPrefix.
type Platform struct {
	store      *SubscriptionStore
	dispatcher *Dispatcher
	validate   TokenValidator
	onCallback CallbackHandler
	mux        *http.ServeMux
}

// New creates a ready-to-use Platform.
// validate:   JWT validator — use middleware.HydraJWTValidator.ValidateToken
// onCallback: called when ORCE POSTs to /callbacks (may be nil)
func New(store *SubscriptionStore, dispatcher *Dispatcher, validate TokenValidator, onCallback CallbackHandler) *Platform {
	p := &Platform{
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
// GET  /webhooks   — authenticated
// POST /webhooks   — authenticated
// DELETE /webhooks/{id} — authenticated
// POST /callbacks  — authenticated
func (p *Platform) routes() {
	p.mux.HandleFunc("GET /health", p.health)
	p.mux.HandleFunc("GET /events", p.auth(p.listEvents))
	p.mux.HandleFunc("GET /webhooks", p.auth(p.listWebhooks))
	p.mux.HandleFunc("POST /webhooks", p.auth(p.subscribe))
	p.mux.HandleFunc("DELETE /webhooks/{id}", p.auth(p.unsubscribe))
	p.mux.HandleFunc("POST /callbacks", p.auth(p.callback))
	p.mux.HandleFunc("GET /deliveries", p.auth(p.listDeliveries))
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

		holderDID, err := p.validate(r.Context(), token)
		if err != nil {
			jsonError(w, http.StatusUnauthorized, "invalid token: "+err.Error())
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyHolderDID{}, holderDID)
		next(w, r.WithContext(ctx))
	}
}

type ctxKeyHolderDID struct{}

func holderDIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyHolderDID{}).(string)
	return v
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
	jsonOK(w, p.store.ListAll())
}

// GET /deliveries — recent notification outcomes (status code / error /
// acknowledged), the monitoring surface for DCS-FR-TR-22 subscribers.
func (p *Platform) listDeliveries(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, p.store.ListDeliveries())
}

// POST /webhooks
func (p *Platform) subscribe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Event       string `json:"event"`
		CallbackURL string `json:"callback_url"`
		Secret      string `json:"secret,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Event == "" || req.CallbackURL == "" {
		jsonError(w, http.StatusBadRequest, "event and callback_url are required")
		return
	}
	if !isKnownEvent(req.Event) {
		jsonError(w, http.StatusBadRequest, "unknown event: "+req.Event)
		return
	}

	sub := p.store.Add(req.Event, req.CallbackURL, req.Secret)
	log.Printf("webhookplatform: new subscription id=%s event=%s url=%s caller=%s",
		sub.ID, sub.Event, sub.CallbackURL, holderDIDFromCtx(r.Context()))

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, sub)
}

// DELETE /webhooks/{id}
func (p *Platform) unsubscribe(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !p.store.Delete(id) {
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

	pending, ok := p.store.ResolvePending(req.EventID)
	if !ok {
		jsonError(w, http.StatusNotFound, "unknown event_id")
		return
	}

	log.Printf("webhookplatform: callback received event_id=%s event=%s did=%s status=%s caller=%s",
		req.EventID, pending.Event, pending.DID, req.Status, holderDIDFromCtx(r.Context()))

	if p.onCallback != nil {
		go p.onCallback(r.Context(), pending, req.Status, req.Result)
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
