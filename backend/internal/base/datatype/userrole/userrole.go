// Package userrole defines the individual end-user roles used for local RBAC
// (checked via UserRoles.HasRoles in command/query handlers). This is a
// separate authorization layer from peer-scoped task ownership: a contract's
// Responsible.Approvers/Reviewers/Negotiators are peer DIDs (which DCS
// instance is responsible for a task), while UserRole governs which
// individual, locally authenticated user may act on behalf of that peer.
package userrole

import (
	"fmt"
)

type UserRole string

const (
	TemplateCreator  UserRole = "Template Creator"
	TemplateReviewer UserRole = "Template Reviewer"
	TemplateApprover UserRole = "Template Approver"
	TemplateManager  UserRole = "Template Manager"

	ContractCreator    UserRole = "Contract Creator"
	ContractReviewer   UserRole = "Contract Reviewer"
	ContractApprover   UserRole = "Contract Approver"
	ContractManager    UserRole = "Contract Manager"
	ContractNegotiator UserRole = "Contract Negotiator"
	ContractSigner     UserRole = "Contract Signer"
	ContractObserver   UserRole = "Contract Observer"

	ArchiveManager      UserRole = "Archive Manager"
	Auditor             UserRole = "Auditor"
	SystemAdministrator UserRole = "Sys. Administrator"
	ComplianceOfficer   UserRole = "Compliance Officer"
	IntegrationManager  UserRole = "Integration Manager"

	ProcessOrchestrator UserRole = "Process Orchestrator"
	Validator           UserRole = "Validator"

	SystemContractCreator  UserRole = "Sys. Contract Creator"
	SystemContractReviewer UserRole = "Sys. Contract Reviewer"
	SystemContractApprover UserRole = "Sys. Contract Approver"
	SystemContractManager  UserRole = "Sys. Contract Manager"
	SystemContractSigner   UserRole = "Sys. Contract Signer"
	ContractTargetSystem   UserRole = "Contract Target System"
	// SystemAuditor extends the SRS System User classes (SRS §2.4 Table 5, all
	// of which are contract-oriented) with the read-only integrity role an
	// external notary needs: it may read the audit trail's tamper-evidence
	// surface and nothing else. See ADR-16.
	SystemAuditor UserRole = "Sys. Auditor"
)

func NewUserRole(s string) (UserRole, error) {
	ts := UserRole(s)
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid user role state: %s", s)
	}
	return ts, nil
}

// IsValid checks if the UserRole is a valid role
func (r UserRole) IsValid() bool {
	switch r {
	case TemplateCreator, TemplateReviewer, TemplateApprover, TemplateManager,
		ContractCreator, ContractReviewer, ContractApprover, ContractManager,
		ContractNegotiator, ContractSigner, ContractObserver,
		ArchiveManager, Auditor, SystemAdministrator, ComplianceOfficer, IntegrationManager,
		ProcessOrchestrator, Validator,
		SystemContractCreator, SystemContractReviewer, SystemContractApprover,
		SystemContractManager, SystemContractSigner, ContractTargetSystem,
		SystemAuditor:
		return true
	}
	return false
}

// String returns the string representation of the UserRole
func (r UserRole) String() string {
	return string(r)
}

// IsSystemRole returns true if the role is a system/automated role
func (r UserRole) IsSystemRole() bool {
	switch r {
	case SystemContractCreator, SystemContractReviewer, SystemContractApprover,
		SystemContractManager, SystemContractSigner, ContractTargetSystem,
		SystemAuditor:
		return true
	}
	return false
}

// IsHumanRole returns true if the role is a human user role
func (r UserRole) IsHumanRole() bool {
	return r.IsValid() && !r.IsSystemRole()
}

type UserRoles []UserRole

func (r UserRoles) HasRoles(requiredRoles ...UserRole) bool {
	for _, requiredRole := range requiredRoles {
		for _, role := range r {
			if role == requiredRole {
				return true
			}
		}
	}
	return false
}
