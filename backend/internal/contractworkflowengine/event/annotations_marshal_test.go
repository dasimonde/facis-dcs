package event

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSubmitEventMarshalsAnnotations(t *testing.T) {
	evt := SubmitEvent{
		DID: "x", Comments: []string{"c"}, Annotations: []BlockAnnotation{{BlockID: "b1", Comment: "Use USD."}},
		OccurredAt: time.Now().UTC(), PreviousState: "SUBMITTED", NewState: "NEGOTIATION",
	}

	b, err := json.Marshal(evt)
	if err != nil {
		t.Fatal(err)
	}

	var m map[string]any
	_ = json.Unmarshal(b, &m)
	anns, ok := m["annotations"].([]any)

	if !ok || len(anns) != 1 {
		t.Fatalf("annotations missing in %s", string(b))
	}
}
