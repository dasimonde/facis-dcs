# ADR-39: Events are delivered through JetStream durable consumers

Status: Accepted (2026-10-31).

## Context

`DCS-NFR-SF-01` requires that the DCS keeps its work through a restart. A
domain command writes its event into `outbox_events` in its own transaction,
and the outbox processor publishes the event on the NATS subject `dcs`
(`internal/base/event/outboxprocessor.go`). Four consumers do work for each
event: the webhook dispatcher (`dcs.webhook`), the PDF regeneration
(`dcs.pdfgeneration`), the automatic deployment (`dcs.deploy`) and the
DCS-to-DCS synchronizer (`dcs.dcstodcs`). Each consumer runs in every backend
replica and must do its work once per event, also when no replica of it runs
at the moment of the publish.

The XFSC cloud-event-provider (v0.1.5) subscribes on core NATS. A queue
subscription there keeps nothing for a subscriber that does not run. Its
JetStream mode binds the stream to the subjects `<stream>.*` and acknowledges
every event whatever the handler does, because the handler returns no result.

## Decision

- The backend uses nats.go JetStream directly
  (`internal/base/event/cloudeventprovider.go`). The publisher creates the
  stream `DCS_EVENTS` on the subject `dcs`, file storage, events kept for
  7 days. A publish returns when the stream has stored the event, and only
  then does the outbox mark it published.
- The message is the structured-mode JSON CloudEvent of the CloudEvents NATS
  binding. Core NATS subscribers on `dcs` (the live dashboard feed and the
  debug event logger, which run in every replica and keep nothing) receive the
  same message through the provider.
- The CloudEvent id is derived from the outbox row (id and creation time), so
  every publish of one outbox event has the same id. It is also the JetStream
  message id: the stream stores a republished event once within its duplicate
  window (2 minutes).
- Each consumer group has one durable pull consumer (`dcs-webhook`,
  `dcs-pdfgeneration`, `dcs-deploy`, `dcs-dcstodcs`) with explicit
  acknowledgement. The replicas of a group share it, so each event goes to one
  replica. The consumer keeps its position while no replica runs, and the
  events published meanwhile are delivered when a replica starts. A new
  consumer starts at the first event in the stream.
- A handler that returns an error gets the event again after 1 s, 2 s, 4 s, ...
  (at most 256 s), at most 10 deliveries. An event that is not acknowledged
  within 2 minutes (longer than every handler timeout) is delivered again.
- The NATS connections reconnect without a limit.
- The chart runs NATS with JetStream and its store on the
  PersistentVolumeClaim `<release>-nats` (`charts/nats`). An external NATS
  must have JetStream with a persistent store.

## Consequences

JetStream delivers at least once, so every consumer handles a second delivery
of an event:

| Consumer | Redelivery | Error returned for |
|---|---|---|
| Webhooks | The CloudEvent id is the webhook event id (`webhook-id`). `webhook_dispatches` has it as primary key, and an id that is recorded already is not dispatched again. | loading the subscribers, recording the dispatch |
| PDF regeneration | Each handler compares the stored PDF state with the record and does nothing when the PDF is current; regenerations of one entity are serialized by an advisory lock. | every failed regeneration; the retry pass (`retryPendingRegenerations`) continues after the last delivery |
| Automatic deployment | The CloudEvent id is the correlation id of the deployment. A deployment with that id makes `Handle` return `ErrAlreadyDispatched`, and nothing is sent to the target. | failures other than the operator outcomes (no target, target not registered or disabled, signing incomplete, gate blocked, contract not SIGNED) |
| DCS-to-DCS | A ship skips the peers that hold the revision (`contract_pdf_deliveries`), settlements and negotiation messages are re-sent only while undelivered. | none: the ship records its failure for the sync-fail scheduler |

- Webhook delivery is at least once. The dispatch and one delivery row per
  subscription (`webhook_deliveries`, with the signed payload) are written in
  one transaction before the first request, each due again one minute later.
  A replica that stops before that transaction leaves the event to the
  redelivery. A replica that stops after it leaves the rows due, and the
  retry job (`Dispatcher.RetryDue`, every minute, one replica at a time) sends
  them. A delivery that the subscriber does not answer with 2xx is sent again
  after 1, 2, 4, ... minutes (at most 1 hour), at most 10 attempts. A
  subscriber can receive one event twice (the attempt was sent but its
  outcome not recorded); it recognises the repeat by `webhook-id`.
- Events can be handled out of order: the replicas and the handler goroutines
  work in parallel, and a redelivered event comes after later ones. Every
  handler reads the current state of the entity instead of the event data.
- A consumer group that is down for longer than 7 days loses the events older
  than that. An event that fails 10 times stays in the stream and is not
  delivered again to that group; the log names it.
- Tests: `TestEachEventIsHandledOncePerConsumerGroup`,
  `TestEventPublishedWhileTheConsumerIsDownIsDeliveredWhenItStarts`,
  `TestFailedEventIsRedelivered` (`internal/base/event`, in-process NATS
  server with JetStream), `TestRepublishedEventKeepsItsCloudEventID`,
  `TestRedeliveredEventIsDispatchedOnce`,
  `TestARecordedDeliveryIsSentAfterTheReplicaStopped`,
  `TestADeliveryIsRetriedUntilTheSubscriberAcceptsIt`,
  `TestPostgresDeliveriesAreSharedAcrossReplicas`,
  `TestRedeliveredDeployEventDispatchesNothing`,
  `TestAutoDeployIsIdempotentPerEvent`.
