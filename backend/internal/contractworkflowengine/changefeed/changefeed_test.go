package changefeed

import (
	"testing"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
	"github.com/stretchr/testify/require"
)

func busEvent(t *testing.T, eventType, data string) cloudevent.Event {
	t.Helper()
	evt := cloudevent.New()
	evt.SetID("1")
	evt.SetSource("dcs")
	evt.SetType(eventType)
	require.NoError(t, evt.SetData(cloudevent.ApplicationJSON, []byte(data)))
	return evt
}

// DCS-FR-CWE-24: a contract change reaches every subscriber; a read event
// does not, so a dashboard that refetches after a change starts no loop.
func TestFeedPassesChangesAndDropsReads(t *testing.T) {
	feed := New()
	first, stopFirst := feed.Subscribe()
	second, stopSecond := feed.Subscribe()
	defer stopSecond()

	feed.Handle(busEvent(t, "RETRIEVE_ALL_CONTRACTS", `{"did":"did:contract:1"}`))
	feed.Handle(busEvent(t, "SEARCH_CONTRACT", `{"did":"did:contract:1"}`))
	feed.Handle(busEvent(t, "SUBMIT_CONTRACT", `{"did":"did:contract:1"}`))

	for _, ch := range []<-chan Change{first, second} {
		require.Equal(t, Change{DID: "did:contract:1", EventType: "SUBMIT_CONTRACT"}, <-ch)
		require.Empty(t, ch)
	}

	stopFirst()
	feed.Handle(busEvent(t, "TERMINATE_CONTRACT", `{"did":"did:contract:1"}`))
	require.Empty(t, first)
	require.Equal(t, "TERMINATE_CONTRACT", (<-second).EventType)
}

// DCS-FR-CWE-24: the deployment acknowledgement that activates a contract
// reaches the dashboards.
func TestFeedPassesTheActivation(t *testing.T) {
	feed := New()
	changes, stop := feed.Subscribe()
	defer stop()

	feed.Handle(busEvent(t, "ACTIVATE_CONTRACT", `{"did":"did:contract:1"}`))

	require.Len(t, changes, 1)
	require.Equal(t, Change{DID: "did:contract:1", EventType: "ACTIVATE_CONTRACT"}, <-changes)
}

// DCS-FR-CWE-24, DCS-FR-CWE-27: a change that a counterparty instance sent
// (an offer, a counteroffer, a signed copy) reaches the dashboards.
func TestAReceivedPeerExchangeIsStreamed(t *testing.T) {
	feed := New()
	changes, stop := feed.Subscribe()
	defer stop()

	feed.Handle(busEvent(t, "PEER_EXCHANGE", `{"did":"did:contract:1","exchange":"post_pdf","direction":"RECEIVED"}`))

	require.Len(t, changes, 1)
	require.Equal(t, Change{DID: "did:contract:1", EventType: "PEER_EXCHANGE"}, <-changes)
}

// DCS-FR-CWE-24: a subscriber that falls behind loses changes, so its
// subscription ends and it reloads; the other subscribers go on.
func TestASubscriberThatFallsBehindIsClosed(t *testing.T) {
	feed := New()
	slow, stopSlow := feed.Subscribe()
	defer stopSlow()
	for i := 0; i < cap(slow)+1; i++ {
		feed.Publish(Change{DID: "did:contract:1", EventType: "UPDATE_CONTRACT"})
	}
	for range cap(slow) {
		<-slow
	}
	select {
	case _, open := <-slow:
		require.False(t, open, "a subscriber that lost a change is closed")
	default:
		t.Fatal("a subscriber that lost a change is closed")
	}

	fresh, stopFresh := feed.Subscribe()
	defer stopFresh()
	feed.Publish(Change{DID: "did:contract:2", EventType: "UPDATE_CONTRACT"})
	require.Equal(t, "did:contract:2", (<-fresh).DID)
}
