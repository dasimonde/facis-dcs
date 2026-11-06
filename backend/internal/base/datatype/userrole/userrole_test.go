package userrole

import "testing"

// System Auditor is a system role, not a human one: it belongs to a machine
// notary and must never show up where human roles are listed (ADR-16).
func TestSystemAuditorIsAValidSystemRole(t *testing.T) {
	if !SystemAuditor.IsValid() {
		t.Fatal("Sys. Auditor is not accepted as a role")
	}
	if !SystemAuditor.IsSystemRole() {
		t.Fatal("Sys. Auditor is not classified as a system role")
	}
	if SystemAuditor.IsHumanRole() {
		t.Fatal("Sys. Auditor is classified as a human role")
	}
}

// The human Auditor stays human: the two are separate roles, and holding one
// must never imply the other.
func TestAuditorAndSystemAuditorAreDistinct(t *testing.T) {
	if Auditor == SystemAuditor {
		t.Fatal("Auditor and Sys. Auditor are the same value")
	}
	if !Auditor.IsHumanRole() {
		t.Fatal("Auditor stopped being a human role")
	}
	if Auditor.IsSystemRole() {
		t.Fatal("Auditor became a system role")
	}
}

// A machine identity holds system roles only, so the webhook management role
// for integrations must be one (DCS-NFR-SQ-08).
func TestSystemIntegrationManagerIsASystemRole(t *testing.T) {
	if !SystemIntegrationManager.IsValid() || !SystemIntegrationManager.IsSystemRole() || SystemIntegrationManager.IsHumanRole() {
		t.Fatal("Sys. Integration Manager is not a valid system role")
	}
	if r, err := NewUserRole("Sys. Integration Manager"); err != nil || r != SystemIntegrationManager {
		t.Fatalf("NewUserRole = %q, %v", r, err)
	}
}
