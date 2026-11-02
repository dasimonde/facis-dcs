package webhookplatform

import (
	"context"
	"encoding/json"
	"log"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"

	"digital-contracting-service/internal/base/event"
)

func StartNATSBridge(subClient *event.DurableSubClient, d *Dispatcher) error {
	return subClient.Subscribe(func(evt cloudevent.Event) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return handleNATSEvent(ctx, evt, d)
	})
}

// handleNATSEvent dispatches one event. The CloudEvent id is the webhook event
// id, so a redelivered event is not dispatched a second time. It returns an
// error only when a redelivery can succeed.
func handleNATSEvent(ctx context.Context, evt cloudevent.Event, d *Dispatcher) error {
	eventType := evt.Type()
	if _, known := DCSEventMap[eventType]; !known {
		return nil
	}

	// The outbox publisher passes the domain event straight through as
	// json.RawMessage (cloudeventprovider.go: marshalling a RawMessage is the
	// identity), so the CloudEvent data IS the domain event object.
	rawPayload := evt.Data()

	var envelope struct {
		DID string `json:"did"`
	}
	if err := json.Unmarshal(rawPayload, &envelope); err != nil {
		log.Printf("webhookplatform: unmarshal DID from %s payload: %v", eventType, err)
		return nil
	}

	return d.DispatchFromDCS(ctx, evt.ID(), eventType, envelope.DID, json.RawMessage(rawPayload))
}
