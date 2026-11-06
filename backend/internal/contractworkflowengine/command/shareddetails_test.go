package command

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
)

func TestParseShippedSharedDetailsReadsAllSharedFields(t *testing.T) {
	payload := []byte(`{
		"@type":"dcs:Contract",
		"dcs:metadata":{
			"@type":"dcs:ContractMetadata",
			"dcs:title":"Peer Offer",
			"dcs:description":"Full description",
			"dcs:startDate":"2026-11-01T00:00:00Z",
			"dcs:expirationDate":"2027-11-01T00:00:00Z",
			"dcs:expirationNoticePeriod":30,
			"dcs:expirationPolicy":"RENEWAL"
		}
	}`)

	got := parseShippedSharedDetails(payload)
	require.NotNil(t, got.Name)
	require.Equal(t, "Peer Offer", *got.Name)
	require.NotNil(t, got.Description)
	require.Equal(t, "Full description", *got.Description)
	require.NotNil(t, got.StartDate)
	require.True(t, got.StartDate.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)))
	require.NotNil(t, got.ExpDate)
	require.True(t, got.ExpDate.Equal(time.Date(2027, 11, 1, 0, 0, 0, 0, time.UTC)))
	require.NotNil(t, got.ExpNoticePeriod)
	require.Equal(t, 30, *got.ExpNoticePeriod)
	require.NotNil(t, got.ExpPolicy)
	require.Equal(t, "RENEWAL", *got.ExpPolicy)
}

func TestParseShippedSharedDetailsLeavesMissingFieldsNil(t *testing.T) {
	payload := []byte(`{"dcs:metadata":{"dcs:title":"Title Only"}}`)
	got := parseShippedSharedDetails(payload)
	require.NotNil(t, got.Name)
	require.Equal(t, "Title Only", *got.Name)
	require.Nil(t, got.Description)
	require.Nil(t, got.StartDate)
	require.Nil(t, got.ExpDate)
	require.Nil(t, got.ExpPolicy)
	require.Nil(t, got.ExpNoticePeriod)
}

func TestStampSharedDetailsIntoContractDataOverwritesEmbeddedTitleFromRow(t *testing.T) {
	// Policy-fixture updates ship a full document whose dcs:title is a fixture
	// label ("BDD ODRL Soundness Contract") while contracts.name stays the
	// template name. Stamping the row name must win so signing's consistency
	// gate does not refuse.
	raw := datatype.JSON(`{"@type":"dcs:Contract","dcs:metadata":{"@type":"dcs:ContractMetadata","dcs:title":"BDD ODRL Soundness Contract"}}`)
	rowName := "BDD Contract Source Template"

	stamped, changed, err := stampSharedDetailsIntoContractData(&raw, sharedContractDetails{Name: &rowName})
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, AssertSharedDetailsConsistent(&rowName, nil, nil, nil, stamped))
}

func TestStampSharedDetailsIntoContractDataWritesMetadata(t *testing.T) {
	raw := datatype.JSON(`{"@type":"dcs:Contract","dcs:metadata":{"@type":"dcs:ContractMetadata","dcs:title":"Old"}}`)
	start := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	exp := time.Date(2027, 12, 1, 0, 0, 0, 0, time.UTC)
	notice := 14
	policy := "TERMINATION"
	name := "New Title"
	desc := "New description"

	stamped, changed, err := stampSharedDetailsIntoContractData(&raw, sharedContractDetails{
		Name:            &name,
		Description:     &desc,
		StartDate:       &start,
		ExpDate:         &exp,
		ExpPolicy:       &policy,
		ExpNoticePeriod: &notice,
	})
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, stamped)

	var doc map[string]any
	require.NoError(t, json.Unmarshal([]byte(*stamped), &doc))
	meta := doc["dcs:metadata"].(map[string]any)
	require.Equal(t, "New Title", meta["dcs:title"])
	require.Equal(t, "New description", meta["dcs:description"])
	require.Equal(t, map[string]any{
		"@value": "2026-12-01T00:00:00Z",
		"@type":  "xsd:dateTime",
	}, meta["dcs:startDate"])
	require.Equal(t, map[string]any{
		"@value": "2027-12-01T00:00:00Z",
		"@type":  "xsd:dateTime",
	}, meta["dcs:expirationDate"])
	require.EqualValues(t, 14, meta["dcs:expirationNoticePeriod"])
	require.Equal(t, "TERMINATION", meta["dcs:expirationPolicy"])

	parsed := parseShippedSharedDetails([]byte(*stamped))
	require.Equal(t, name, *parsed.Name)
	require.Equal(t, desc, *parsed.Description)
	require.True(t, parsed.StartDate.Equal(start))
	require.True(t, parsed.ExpDate.Equal(exp))
	require.Equal(t, notice, *parsed.ExpNoticePeriod)
	require.Equal(t, policy, *parsed.ExpPolicy)
}

func TestParseShippedSharedDetailsReadsTypedDateLiterals(t *testing.T) {
	payload := []byte(`{
		"dcs:metadata":{
			"dcs:title":"T",
			"dcs:startDate":{"@value":"2026-11-01T00:00:00Z","@type":"xsd:dateTime"},
			"dcs:expirationDate":{"@value":"2027-11-01T00:00:00Z","@type":"xsd:dateTime"}
		}
	}`)
	got := parseShippedSharedDetails(payload)
	require.NotNil(t, got.StartDate)
	require.True(t, got.StartDate.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)))
	require.NotNil(t, got.ExpDate)
	require.True(t, got.ExpDate.Equal(time.Date(2027, 11, 1, 0, 0, 0, 0, time.UTC)))
}

func TestReceivingAPeerContractPersistsSharedDetails(t *testing.T) {
	receiver, _, contracts, _ := receivingSide(t)

	payload := []byte(`{
		"@id":"` + taskContractDID + `",
		"@type":"dcs:Contract",
		"dcs:metadata":{
			"dcs:title":"Shipped Agreement",
			"dcs:description":"Peer description",
			"dcs:startDate":"2026-10-20T00:00:00Z",
			"dcs:expirationDate":"2027-10-20T00:00:00Z",
			"dcs:expirationNoticePeriod":7,
			"dcs:expirationPolicy":"ARCHIVING"
		}
	}`)
	cmd := peerShipCmd()
	cmd.Payload = payload

	require.NoError(t, receiver.Handle(t.Context(), cmd))
	require.NotNil(t, contracts.row)
	require.NotNil(t, contracts.row.Name)
	require.Equal(t, "Shipped Agreement", *contracts.row.Name)
	require.NotNil(t, contracts.row.Description)
	require.Equal(t, "Peer description", *contracts.row.Description)
	require.NotNil(t, contracts.row.StartDate)
	require.NotNil(t, contracts.row.ExpDate)
	require.NotNil(t, contracts.row.ExpNoticePeriod)
	require.Equal(t, 7, *contracts.row.ExpNoticePeriod)
	require.NotNil(t, contracts.row.ExpPolicy)
	require.Equal(t, "ARCHIVING", *contracts.row.ExpPolicy)
}
