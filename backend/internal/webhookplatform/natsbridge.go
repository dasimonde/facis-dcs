package webhookplatform

import (
	"context"
	"encoding/json"
	"log"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"

	"digital-contracting-service/internal/base/event"
)

func StartNATSBridge(subClient *event.CloudEventSubClient, d *Dispatcher) error {
	return subClient.Subscribe(func(evt cloudevent.Event) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		handleNATSEvent(ctx, evt, d)
	})
}

func handleNATSEvent(ctx context.Context, evt cloudevent.Event, d *Dispatcher) {
	eventType := evt.Type()
	if _, known := DCSEventMap[eventType]; !known {
		return
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
		return
	}

	d.DispatchFromDCS(ctx, eventType, envelope.DID, json.RawMessage(rawPayload))
}
