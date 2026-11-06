package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	baseevent "digital-contracting-service/internal/base/event"
)

const pacSnapshotVersion = 1

type pacSnapshotCipher interface {
	Encrypt(context.Context, artifactstore.Scope, []byte) ([]byte, error)
	Decrypt(context.Context, artifactstore.Scope, []byte) ([]byte, error)
	InstanceScope() artifactstore.Scope
}

type pacSnapshotEnvelope struct {
	Version    int                   `json:"version"`
	AuditID    string                `json:"auditId"`
	Scopes     []artifactstore.Scope `json:"scopes"`
	Ciphertext []byte                `json:"ciphertext"`
}

type pacSnapshotPayload struct {
	RequestBytes  []byte `json:"requestBytes"`
	ResponseBytes []byte `json:"responseBytes"`
}

func pacEvidenceScopes(requestBytes []byte, instance artifactstore.Scope) ([]artifactstore.Scope, error) {
	var request struct {
		Scope    string                                      `json:"scope"`
		Evidence map[string][]persistedAuditEvidenceResource `json:"evidence"`
	}
	if err := json.Unmarshal(requestBytes, &request); err != nil {
		return nil, fmt.Errorf("decode snapshot evidence: %w", err)
	}
	resources, ok := request.Evidence[request.Scope]
	if !ok {
		return nil, fmt.Errorf("snapshot lacks scope evidence")
	}
	unique := map[artifactstore.Scope]bool{}
	for _, resource := range resources {
		for _, entry := range resource.AuditTrail {
			erased := len(entry.EventData) == 1 && entry.EventData["erased"] == true
			if (len(entry.EventData) == 0 || erased) && entry.Reason == "" {
				continue
			}
			scope := pacEntryScope(resource.DID, resource.Component, entry, instance)
			if scope != instance {
				unique[scope] = true
			}
		}
	}
	scopes := make([]artifactstore.Scope, 0, len(unique))
	for scope := range unique {
		scopes = append(scopes, scope)
	}
	sort.Slice(scopes, func(i, j int) bool { return scopes[i].AAD() < scopes[j].AAD() })
	return scopes, nil
}

func pacEntryScope(resourceDID, resourceComponent string, entry persistedAuditEvidenceEntry, instance artifactstore.Scope) artifactstore.Scope {
	did := firstNonEmpty(entry.DID, resourceDID)
	return baseevent.ScopeForEvent(datatype.OutboxEvent{Component: firstNonEmpty(entry.Component, resourceComponent), EventType: entry.EventType, DID: &did}, instance)
}

// redactErasedEvidence replaces the content of evidence entries whose resource
// CEK is shredded with the erased marker before the run is sent and sealed. The
// database can still hold such content (archive entry rows, synthetic checks),
// and a snapshot must not carry it past the erasure (DCS-NFR-SEC-13). The entry
// stays in the evidence, so the erasure stays visible.
func redactErasedEvidence(ctx context.Context, cipher pacSnapshotCipher, evidence []*auditEvidenceResource) {
	instance := cipher.InstanceScope()
	shredded := map[artifactstore.Scope]bool{}
	for _, resource := range evidence {
		for _, entry := range resource.AuditTrail {
			if entry == nil {
				continue
			}
			scope := pacEntryScope(resource.Did, resource.Component, persistedAuditEvidenceEntry{DID: stringValue(entry.Did), Component: entry.Component, EventType: entry.EventType}, instance)
			if scope == instance {
				continue
			}
			erased, known := shredded[scope]
			if !known {
				// Decrypt reads the same CEK record that sealing uses, without
				// creating one.
				_, err := cipher.Decrypt(ctx, scope, nil)
				erased = artifactstore.IsShredded(err)
				shredded[scope] = erased
			}
			if erased {
				entry.EventData = map[string]any{"erased": true}
				entry.Reason = nil
			}
		}
	}
}

func validatePACScopes(scopes []artifactstore.Scope) error {
	previous := ""
	for _, scope := range scopes {
		if scope.ID == "" || (scope.Kind != artifactstore.KindContract && scope.Kind != artifactstore.KindTemplate) || scope.AAD() <= previous {
			return fmt.Errorf("invalid PAC snapshot resource scopes")
		}
		previous = scope.AAD()
	}
	return nil
}

// The outer instance encryption authenticates the scope list and run identity.
func sealPACSnapshot(ctx context.Context, cipher pacSnapshotCipher, auditID string, scopes []artifactstore.Scope, payload []byte) ([]byte, error) {
	if auditID == "" {
		return nil, fmt.Errorf("missing PAC snapshot audit identity")
	}
	if err := validatePACScopes(scopes); err != nil {
		return nil, err
	}
	ciphertext := payload
	var err error
	for _, scope := range scopes {
		ciphertext, err = cipher.Encrypt(ctx, scope, ciphertext)
		if err != nil {
			return nil, fmt.Errorf("protect PAC snapshot: %w", err)
		}
	}
	return json.Marshal(pacSnapshotEnvelope{Version: pacSnapshotVersion, AuditID: auditID, Scopes: scopes, Ciphertext: ciphertext})
}

func unsealPACSnapshot(ctx context.Context, cipher pacSnapshotCipher, auditID string, envelopeBytes []byte) ([]byte, []artifactstore.Scope, error) {
	var envelope pacSnapshotEnvelope
	if err := json.Unmarshal(envelopeBytes, &envelope); err != nil {
		return nil, nil, fmt.Errorf("decode PAC snapshot envelope: %w", err)
	}
	if envelope.Version != pacSnapshotVersion || envelope.AuditID != auditID || auditID == "" {
		return nil, nil, fmt.Errorf("PAC snapshot version or identity mismatch")
	}
	if err := validatePACScopes(envelope.Scopes); err != nil {
		return nil, nil, err
	}
	payload := envelope.Ciphertext
	for i := len(envelope.Scopes) - 1; i >= 0; i-- {
		var err error
		payload, err = cipher.Decrypt(ctx, envelope.Scopes[i], payload)
		if err != nil {
			return nil, nil, fmt.Errorf("unprotect PAC snapshot: %w", err)
		}
	}
	return payload, envelope.Scopes, nil
}

func equalPACScopes(a, b []artifactstore.Scope) bool { return slices.Equal(a, b) }
