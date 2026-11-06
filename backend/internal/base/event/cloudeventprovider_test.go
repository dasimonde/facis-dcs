package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"
)

// jetStreamServer starts an in-process NATS server with JetStream.
func jetStreamServer(t *testing.T) string {
	t.Helper()
	ns, err := server.NewServer(&server.Options{
		Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true,
		JetStream: true, StoreDir: t.TempDir(),
	})
	require.NoError(t, err)
	go ns.Start()
	t.Cleanup(ns.Shutdown)
	require.True(t, ns.ReadyForConnections(5*time.Second))
	return ns.ClientURL()
}

func publisher(t *testing.T, url string) *CloudEventPubClient {
	t.Helper()
	pub, err := NewNatsPubClient("dcs", url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pub.Close() })
	return pub
}

// durable starts one replica of consumer group group; f sees every event the
// replica receives.
func durable(t *testing.T, url, group string, f func(cloudevent.Event) error) *DurableSubClient {
	t.Helper()
	c, err := NewDurableSubClient("dcs", url, group)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	go func() { _ = c.Subscribe(f) }()
	return c
}

// counter counts the deliveries per CloudEvent id.
type counter struct {
	mu   sync.Mutex
	seen map[string]int
}

func (c *counter) handle(evt cloudevent.Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen == nil {
		c.seen = map[string]int{}
	}
	c.seen[evt.ID()]++
	return nil
}

func (c *counter) snapshot() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[string]int{}
	for k, v := range c.seen {
		out[k] = v
	}
	return out
}

// DCS-NFR-PER-02: the replicas of one instance share the durable consumer of a
// group, so a webhook, a PDF regeneration or an auto-deploy runs once per
// event and not once per replica. A different consumer group still sees every
// event, and a core subscriber (live dashboard) receives the same CloudEvent.
func TestEachEventIsHandledOncePerConsumerGroup(t *testing.T) {
	url := jetStreamServer(t)
	pub := publisher(t, url)

	var webhook, deploy counter
	durable(t, url, "dcs.webhook", webhook.handle)
	durable(t, url, "dcs.webhook", webhook.handle)
	durable(t, url, "dcs.webhook", webhook.handle)
	durable(t, url, "dcs.deploy", deploy.handle)

	var broadcast atomic.Int32
	core, err := NewNatsSubClient("dcs", url)
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })
	go func() {
		_ = core.Subscribe(func(evt cloudevent.Event) {
			var data struct {
				DID string `json:"did"`
			}
			if evt.DataAs(&data) == nil && data.DID == "x" {
				broadcast.Add(1)
			}
		})
	}()
	time.Sleep(200 * time.Millisecond) // the core subscription opens asynchronously

	const events = 10
	for i := range events {
		require.NoError(t, pub.Publish(fmt.Sprint("id-", i), "test", "CREATE_CONTRACT", json.RawMessage(`{"did":"x"}`)))
	}

	once := func(c *counter) bool {
		s := c.snapshot()
		if len(s) != events {
			return false
		}
		for _, n := range s {
			if n != 1 {
				return false
			}
		}
		return true
	}
	require.Eventually(t, func() bool { return once(&webhook) && once(&deploy) }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(200 * time.Millisecond)
	require.True(t, once(&webhook), "each event must reach one webhook replica once: %v", webhook.snapshot())
	require.True(t, once(&deploy), "the deploy group must see each event once: %v", deploy.snapshot())
	require.EqualValues(t, events, broadcast.Load())
}

// DCS-NFR-SF-01: an event published while no replica of a consumer group runs
// is kept, and delivered once when a replica starts. A republish of the same
// outbox event (same id) is stored once.
func TestEventPublishedWhileTheConsumerIsDownIsDeliveredWhenItStarts(t *testing.T) {
	url := jetStreamServer(t)
	pub := publisher(t, url)

	var before counter
	stopped := durable(t, url, "dcs.webhook", before.handle)
	require.NoError(t, stopped.Close())

	require.NoError(t, pub.Publish("while-down", "test", "CREATE_CONTRACT", json.RawMessage(`{"did":"x"}`)))
	require.NoError(t, pub.Publish("while-down", "test", "CREATE_CONTRACT", json.RawMessage(`{"did":"x"}`)))

	var after counter
	durable(t, url, "dcs.webhook", after.handle)
	durable(t, url, "dcs.webhook", after.handle)

	require.Eventually(t, func() bool { return after.snapshot()["while-down"] == 1 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	require.Equal(t, map[string]int{"while-down": 1}, after.snapshot())
	require.Empty(t, before.snapshot())
}

// DCS-NFR-SF-01: an event whose handler fails is delivered again, and an
// acknowledged event is not.
func TestFailedEventIsRedelivered(t *testing.T) {
	url := jetStreamServer(t)
	pub := publisher(t, url)

	var attempts atomic.Int32
	durable(t, url, "dcs.pdfgeneration", func(cloudevent.Event) error {
		if attempts.Add(1) == 1 {
			return errors.New("pdf-core unavailable")
		}
		return nil
	})

	require.NoError(t, pub.Publish("retry-me", "test", "CREATE_CONTRACT", json.RawMessage(`{"did":"x"}`)))

	require.Eventually(t, func() bool { return attempts.Load() == 2 }, 10*time.Second, 20*time.Millisecond)
	time.Sleep(1500 * time.Millisecond)
	require.EqualValues(t, 2, attempts.Load(), "the acknowledged second delivery must be the last")
}
