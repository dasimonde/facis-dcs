package base

import "testing"

func TestIsAuditVisibleEventType(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		want      bool
	}{
		{name: "retrieve by id is hidden", eventType: "RETRIEVE_CONTRACT_TEMPLATE_BY_ID", want: false},
		{name: "retrieve all is hidden", eventType: "RETRIEVE_ALL_CONTRACTS", want: false},
		{name: "retrieve archived is hidden", eventType: "RETRIEVE_ARCHIVED_CONTRACTS", want: false},
		{name: "search is hidden", eventType: "SEARCH_CONTRACT_TEMPLATE", want: false},
		{name: "archive store is visible", eventType: "STORE_ARCHIVED_CONTRACT", want: true},
		{name: "archive audit summary is visible", eventType: "ARCHIVE_ENTRY_AUDIT_SUMMARY", want: true},
		{name: "policy finding is visible", eventType: "TEMPLATE_POLICY_AUDIT_FINDING", want: true},
		{name: "lifecycle event is visible", eventType: "APPROVE_CONTRACT_TEMPLATE", want: true},
		{name: "case and whitespace are normalized", eventType: " retrieve_contract_by_id ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAuditVisibleEventType(tt.eventType); got != tt.want {
				t.Fatalf("IsAuditVisibleEventType(%q) = %v, want %v", tt.eventType, got, tt.want)
			}
		})
	}
}
