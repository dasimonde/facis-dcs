// Package event implements the base-level transactional outbox pattern that
// every domain's command/query handlers use for audit logging: handlers
// persist a domain event in the outbox table within their own DB
// transaction (see Create/eventpersistence.go); OutboxProcessor then
// republishes each event on the NATS event bus as a CloudEvent (this file)
// for other domains (pdfgeneration, webhookplatform, dcstodcs,
// contractworkflowengine/deployevent) to subscribe to, and separately,
// asynchronously anchors it to IPFS/TSA as a tamper-evident, hash-chained
// audit-trail entry (see outboxprocessor.go).
//
// Delivery is durable (ADR-39): the JetStream stream EventStream keeps every
// event, and each consumer group reads it through its own durable consumer.
package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	cloudeventprovider "github.com/eclipse-xfsc/cloud-event-provider"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// EventStream is the JetStream stream that keeps the events of the event bus
// topic.
const EventStream = "DCS_EVENTS"

const (
	// eventRetention is how long the stream keeps an event, so a consumer group
	// that is down for less than this receives the events it missed.
	eventRetention = 7 * 24 * time.Hour
	// ackWait is longer than the longest handler timeout (60 s): an event is
	// redelivered without a nak only when its handler did not finish.
	ackWait = 2 * time.Minute
	// maxDeliver bounds the deliveries of one event to one consumer group.
	maxDeliver = 10
	// natsRequestTimeout bounds one JetStream API call or publish.
	natsRequestTimeout = 10 * time.Second
)

// connect opens a NATS connection that reconnects without limit, so the
// clients come back when the server comes back.
func connect(natsURL string) (*nats.Conn, jetstream.JetStream, error) {
	nc, err := nats.Connect(natsURL, nats.MaxReconnects(-1))
	if err != nil {
		return nil, nil, fmt.Errorf("could not connect to NATS: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, nil, fmt.Errorf("could not open JetStream: %w", err)
	}
	return nc, js, nil
}

// ensureStream creates or updates EventStream. The server keeps it on disk.
func ensureStream(ctx context.Context, js jetstream.JetStream, topic string) (jetstream.Stream, error) {
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     EventStream,
		Subjects: []string{topic},
		Storage:  jetstream.FileStorage,
		MaxAge:   eventRetention,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create JetStream stream %s: %w", EventStream, err)
	}
	return stream, nil
}

type CloudEventPubClient struct {
	nc    *nats.Conn
	js    jetstream.JetStream
	topic string
}

func (c CloudEventPubClient) Close() error {
	c.nc.Close()
	return nil
}

// Publish publishes a structured-mode JSON CloudEvent, the encoding of the
// CloudEvents NATS binding, and returns when the stream has stored it. id is
// the CloudEvent id and the JetStream message id: the stream stores a second
// publish of the same id within its duplicate window once.
func (c CloudEventPubClient) Publish(id, eventSource, eventType string, payload json.RawMessage) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("could not marshal payload: %w", err)
	}
	evt := cloudevent.New()
	evt.SetID(id)
	evt.SetSource(eventSource)
	evt.SetType(eventType)
	if err := evt.SetData(cloudevent.ApplicationJSON, json.RawMessage(data)); err != nil {
		return fmt.Errorf("could not create cloud event: %w", err)
	}
	body, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("could not encode cloud event: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), natsRequestTimeout)
	defer cancel()
	_, err = c.js.Publish(ctx, c.topic, body, jetstream.WithMsgID(id))
	return err
}

func NewNatsPubClient(topic string, natsURL string) (*CloudEventPubClient, error) {
	log.Println("nats: create publisher client for topic", topic)
	nc, js, err := connect(natsURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), natsRequestTimeout)
	defer cancel()
	if _, err := ensureStream(ctx, js, topic); err != nil {
		nc.Close()
		return nil, err
	}
	return &CloudEventPubClient{nc, js, topic}, nil
}

// DurableSubClient reads the events of one consumer group from EventStream.
// The replicas of one DCS instance share the durable consumer of a group, so
// each event is handled once per group. The consumer keeps its position while
// no replica runs, and the events published meanwhile are delivered when one
// starts.
type DurableSubClient struct {
	ctx      context.Context
	cancel   context.CancelFunc
	nc       *nats.Conn
	consumer jetstream.Consumer
}

func (c *DurableSubClient) Close() error {
	c.cancel()
	c.nc.Close()
	return nil
}

// Subscribe calls f for each event, each in its own goroutine. An event is
// acknowledged when f returns nil. When f returns an error the event is
// redelivered after a delay that doubles with each delivery, at most
// maxDeliver times. Subscribe returns when the client is closed.
func (c *DurableSubClient) Subscribe(f func(evt cloudevent.Event) error) error {
	consumeCtx, err := c.consumer.Consume(func(msg jetstream.Msg) {
		go handleMsg(msg, f)
	})
	if err != nil {
		return fmt.Errorf("could not consume JetStream events: %w", err)
	}
	<-c.ctx.Done()
	consumeCtx.Stop()
	return nil
}

func handleMsg(msg jetstream.Msg, f func(evt cloudevent.Event) error) {
	var evt cloudevent.Event
	if err := json.Unmarshal(msg.Data(), &evt); err != nil {
		log.Printf("nats: discard an event that is not a CloudEvent: %v", err)
		_ = msg.Term()
		return
	}
	if err := f(evt); err != nil {
		delivered := uint64(1)
		if md, mdErr := msg.Metadata(); mdErr == nil {
			delivered = md.NumDelivered
		}
		log.Printf("nats: event %s (%s) failed on delivery %d of %d: %v", evt.ID(), evt.Type(), delivered, maxDeliver, err)
		_ = msg.NakWithDelay(time.Second << min(delivered-1, 8))
		return
	}
	if err := msg.Ack(); err != nil {
		log.Printf("nats: could not acknowledge event %s: %v", evt.ID(), err)
	}
}

// NewDurableSubClient creates or updates the durable consumer of consumer
// group group on EventStream.
func NewDurableSubClient(topic, natsURL, group string) (*DurableSubClient, error) {
	if group == "" {
		return nil, errors.New("a durable subscriber needs a consumer group")
	}
	log.Println("nats: create durable subscriber for topic", topic, "consumer group", group)
	nc, js, err := connect(natsURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), natsRequestTimeout)
	defer cancel()
	stream, err := ensureStream(ctx, js, topic)
	if err != nil {
		nc.Close()
		return nil, err
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		// A durable name cannot contain a dot.
		Durable:       strings.ReplaceAll(group, ".", "-"),
		FilterSubject: topic,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		MaxDeliver:    maxDeliver,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("could not create JetStream consumer %s: %w", group, err)
	}
	subCtx, subCancel := context.WithCancel(context.Background())
	return &DurableSubClient{ctx: subCtx, cancel: subCancel, nc: nc, consumer: consumer}, nil
}

type CloudEventSubClient struct {
	ctx    context.Context
	cancel context.CancelFunc
	topic  string
	client *cloudeventprovider.CloudEventProviderClient
}

func (c CloudEventSubClient) Close() error {
	c.cancel()
	return c.client.Close()
}

func (c CloudEventSubClient) Subscribe(f func(evt cloudevent.Event)) error {
	return c.client.SubCtx(c.ctx, f)
}

// NewNatsSubClient subscribes to topic on core NATS. Every subscriber receives
// every event that is published while it is subscribed, so every replica sees
// it (live dashboard updates, the debug event logger). Nothing is kept for a
// subscriber that is not running: work that must happen once per event uses a
// DurableSubClient.
func NewNatsSubClient(topic, natsURL string) (*CloudEventSubClient, error) {
	log.Println("nats: create subscriber client for topic", topic)
	client, err := cloudeventprovider.New(cloudeventprovider.Config{
		Protocol: cloudeventprovider.ProtocolTypeNats,
		Settings: cloudeventprovider.NatsConfig{
			Url: natsURL,
		},
	}, cloudeventprovider.ConnectionTypeSub, topic)
	if err != nil {
		return nil, errors.New("could not create cloud event provider client")
	}
	ctx, cancel := context.WithCancel(context.Background())

	return &CloudEventSubClient{ctx, cancel, topic, client}, nil
}
