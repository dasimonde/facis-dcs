package event

import (
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
)

// AuditEvent is emitted when the contract is audited
type AuditEvent struct {
	DID           string                      `json:"did"`
	HolderDID     string                      `json:"holder_did"`
	AuditedBy     string                      `json:"audited_by"`
	OccurredAt    time.Time                   `json:"occurred_at"`
	ComponentType componenttype.ComponentType `json:"component_type"`
	Scope         componenttype.ComponentType `json:"scope"`
	UserRoles     userrole.UserRoles          `json:"user_roles"`
	Justification string                      `json:"justification"`
}

// AuditExecutedEvent records the immutable external-executor result committed
// in pac_audit_runs. It intentionally carries hashes and executor provenance,
// while the full validated response remains in the run table.
type AuditExecutedEvent struct {
	AuditID         string    `json:"audit_id"`
	CorrelationID   string    `json:"correlation_id"`
	Scope           string    `json:"scope"`
	DID             string    `json:"did,omitempty"`
	ExecutorID      string    `json:"executor_id"`
	ExecutorVersion string    `json:"executor_version"`
	ExecutedAt      time.Time `json:"executed_at"`
	RequestHash     string    `json:"request_hash"`
	ResponseHash    string    `json:"response_hash"`
}

func (e AuditExecutedEvent) EventType() string { return "PAC_AUDIT_EXECUTED" }

func (e AuditExecutedEvent) GetDID() string {
	if e.DID == "" {
		return "*"
	}
	return e.DID
}

// EventType implements the Event interface.
func (e AuditEvent) EventType() string {
	return eventtype.Audit.String()
}

// GetDID implements the Event interface.
func (e AuditEvent) GetDID() string {
	if e.DID == "" {
		return "*"
	}
	return e.DID
}

// ReportGeneratedEvent is emitted when PACM generates an audit report.
type ReportGeneratedEvent struct {
	ReportID      string             `json:"report_id"`
	Scope         string             `json:"scope"`
	Format        string             `json:"format"`
	DID           string             `json:"did,omitempty"`
	GeneratedBy   string             `json:"generated_by"`
	GeneratedAt   time.Time          `json:"generated_at"`
	ContentHash   string             `json:"report_hash"`
	ContentCID    string             `json:"report_cid"`
	Justification string             `json:"justification"`
	Summary       map[string]int     `json:"summary"`
	HolderDID     string             `json:"holder_did"`
	UserRoles     userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ReportGeneratedEvent) EventType() string {
	return "PAC_REPORT_GENERATED"
}

// GetDID implements the Event interface.
func (e ReportGeneratedEvent) GetDID() string {
	if e.DID == "" {
		return "*"
	}
	return e.DID
}

// ComplianceRisk is one detected policy-adherence violation, embedded in
// ComplianceMonitorEvent so the audit trail records what the sweep flagged.
type ComplianceRisk struct {
	DID        string    `json:"did"`
	RiskType   string    `json:"risk_type"`
	Detail     string    `json:"detail"`
	DetectedAt time.Time `json:"detected_at"`
}

// ComplianceMonitorEvent is emitted for every continuous-monitoring sweep
// (GET /pac/monitor, DCS-IR-PACM-03) — including clean sweeps, so the audit
// trail proves monitoring actually ran, not only that risks were found.
type ComplianceMonitorEvent struct {
	MonitoredBy string             `json:"monitored_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	Risks       []ComplianceRisk   `json:"risks"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ComplianceMonitorEvent) EventType() string {
	return "PAC_COMPLIANCE_MONITOR"
}

// GetDID implements the Event interface.
func (e ComplianceMonitorEvent) GetDID() string {
	return "*"
}

// ComplianceRiskEvent anchors one detected compliance risk against the
// affected contract's PAC audit chain. The sweep-level ComplianceMonitorEvent
// carries no resource DID (GetDID "*") and therefore only enters the global
// chain — per-resource anchoring is what makes a flagged risk visible in a
// PROCESS_AUDIT_AND_COMPLIANCE-scope audit read ("flagged ... for manual
// review", DCS-FR-PACM-03).
type ComplianceRiskEvent struct {
	DID         string             `json:"did"`
	RiskType    string             `json:"risk_type"`
	Detail      string             `json:"detail"`
	MonitoredBy string             `json:"monitored_by"`
	OccurredAt  time.Time          `json:"occurred_at"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e ComplianceRiskEvent) EventType() string {
	return "PAC_COMPLIANCE_RISK"
}

// GetDID implements the Event interface.
func (e ComplianceRiskEvent) GetDID() string {
	return e.DID
}

// IncidentReportEvent anchors one non-compliance finding submitted through
// POST /pac/report (DCS-IR-PACM-04) against the affected contract or
// template's PAC audit chain, mirroring ComplianceRiskEvent's per-resource
// anchoring so a PROCESS_AUDIT_AND_COMPLIANCE-scope audit read can prove the
// finding was recorded, not merely accepted.
type IncidentReportEvent struct {
	DID        string             `json:"did"`
	RiskType   string             `json:"risk_type"`
	Detail     string             `json:"detail"`
	ReportedBy string             `json:"reported_by"`
	OccurredAt time.Time          `json:"occurred_at"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
}

// EventType implements the Event interface.
func (e IncidentReportEvent) EventType() string {
	return "PAC_INCIDENT_REPORT"
}

// GetDID implements the Event interface.
func (e IncidentReportEvent) GetDID() string {
	return e.DID
}

// TrustGateDenialEvent anchors one federation trust-gate rejection (ADR-19):
// either the agreement-credential check (layer 3a: missing, unsigned, or a
// federation-rules hash mismatch) or the local policy-endpoint check (layer
// 3b: a non-2xx response, an unset DCS_TRUST_PDP_URL, or an unreachable
// endpoint), on either the inbound (PostPdf) or outbound (shipContractPDF)
// path. Anchored against the affected contract's PAC audit chain, mirroring
// IncidentReportEvent's per-resource anchoring.
type TrustGateDenialEvent struct {
	DID        string    `json:"did"`
	PeerDID    string    `json:"peer_did"`
	Direction  string    `json:"direction"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e TrustGateDenialEvent) EventType() string {
	return "PAC_TRUST_GATE_DENIAL"
}

// GetDID implements the Event interface.
func (e TrustGateDenialEvent) GetDID() string {
	return e.DID
}
