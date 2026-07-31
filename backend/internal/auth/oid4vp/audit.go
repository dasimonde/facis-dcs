package oid4vp

import (
	"context"

	"goa.design/clue/log"
)

// PresentationAuditRecorder persists presentation outcomes for immutable audit logging.
type PresentationAuditRecorder interface {
	RecordPresentationAudit(ctx context.Context, evt PresentationAuditEvent) error
}

// PresentationAuditEvent captures auth presentation outcomes for immutable audit logging.
type PresentationAuditEvent struct {
	PresentationState string
	Success           bool
	SubjectDID        string
	ParticipantDID    string
	Roles             []string
	ErrorMessage      string
}

var presentationAuditRecorder PresentationAuditRecorder

// ConfigurePresentationAuditRecorder wires audit persistence.
func ConfigurePresentationAuditRecorder(recorder PresentationAuditRecorder) {
	presentationAuditRecorder = recorder
}

// RecordPresentationAudit writes a presentation outcome to the audit trail when configured.
func RecordPresentationAudit(ctx context.Context, evt PresentationAuditEvent) {
	if presentationAuditRecorder == nil {
		return
	}

	err := presentationAuditRecorder.RecordPresentationAudit(ctx, evt)
	if err != nil {
		log.Printf(ctx, "oid4vp presentation audit failed: %v", err)
	}
}
