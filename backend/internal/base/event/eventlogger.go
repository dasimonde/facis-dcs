package event

import (
	"context"
	"encoding/json"
	"fmt"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"goa.design/clue/log"
)

func StartEventLogger(ctx context.Context, client *CloudEventSubClient) {
	eventHandler := func(evt cloudevent.Event) {

		data, err := json.Marshal(evt)
		if err != nil {
			log.Printf(ctx, "Could not marshal event to JSON: %v", err)
		}

		fmt.Printf("received event: %s\n", string(data))
	}
	go func() {
		if err := client.Subscribe(eventHandler); err != nil {
			log.Errorf(ctx, err, "could not start event printer")
		}
	}()
}
