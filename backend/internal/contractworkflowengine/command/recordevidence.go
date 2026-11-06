package command

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/identity"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type Evidence struct {
	EvidenceID      string `json:"evidence_id"`
	DID             string `json:"did"`
	ContractVersion int    `json:"contract_version"`
	RecordedBy      string `json:"recorded_by"`
	RecordedAt      string `json:"recorded_at"`
	Filename        string `json:"filename"`
	Description     string `json:"description"`
	MediaType       string `json:"media_type"`
	SHA256          string `json:"sha256"`
	Content         []byte `json:"content"`
}

var ErrInvalidEvidence = errors.New("invalid evidence: file, filename and description are required and must fit configured limits")
var ErrEvidenceTerminated = errors.New("evidence cannot be recorded for a terminated contract")

// ErrEvidenceCreatorDraftOnly is returned when a Contract Creator without a
// manager role records evidence after the draft left DRAFT or REJECTED. The
// Manager stores evidence for the rest of the lifecycle (DCS-IR-CWE-12).
var ErrEvidenceCreatorDraftOnly = errors.New("a contract creator records evidence only while the contract is a draft")

func (e Evidence) Validate() error {
	if len(e.Content) == 0 || len(e.Content) > conf.EvidenceFileLimit() || strings.TrimSpace(e.Filename) == "" || strings.TrimSpace(e.Description) == "" || len(e.Filename) > conf.EvidenceTextLimit() || len(e.Description) > conf.EvidenceTextLimit() || len(e.MediaType) > conf.EvidenceTextLimit() {
		return ErrInvalidEvidence
	}
	return nil
}
func (e Evidence) Verify(did, id string) error {
	hash := sha256.Sum256(e.Content)
	if e.DID != did || e.EvidenceID != id || e.SHA256 != hex.EncodeToString(hash[:]) {
		return errors.New("evidence integrity check failed")
	}
	return nil
}

type RecordEvidenceCmd struct {
	Evidence Evidence

	DID        string             `json:"did"`
	RecordedBy string             `json:"recorded_by"`
	UpdatedAt  time.Time          `json:"updated_at"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
	CauserDID  string             `json:"causer_did"`
}

type EvidenceRecorder struct {
	Artifacts   *artifactstore.Store
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	DIDDocument identity.DIDDocument
}

func (h *EvidenceRecorder) Handle(ctx context.Context, cmd RecordEvidenceCmd) (string, error) {
	if err := cmd.Evidence.Validate(); err != nil {
		return "", err
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return "", fmt.Errorf("could not read process data: %w", err)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return "", err
	}

	// Optimistic concurrency: reject if the caller's view of the contract is
	// older than what's stored (see package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return "", fmt.Errorf("contract %w, please force synchronisation and reload", base.ErrUpdatedElsewhere)
		}
		return "", fmt.Errorf("contract %w, please reload", base.ErrUpdatedElsewhere)
	}

	if processData.State == contractstate.Terminated.String() {
		return "", ErrEvidenceTerminated
	}
	if !cmd.UserRoles.HasRoles(userrole.ContractManager, userrole.SystemContractManager) &&
		processData.State != contractstate.Draft.String() && processData.State != contractstate.Rejected.String() {
		return "", ErrEvidenceCreatorDraftOnly
	}

	id, err := base.GenerateID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	evidence := cmd.Evidence
	evidence.EvidenceID, evidence.DID = *id, cmd.DID
	evidence.ContractVersion, evidence.RecordedBy = processData.ContractVersion, cmd.RecordedBy
	evidence.RecordedAt = now.Format(time.RFC3339Nano)
	hash := sha256.Sum256(evidence.Content)
	evidence.SHA256 = hex.EncodeToString(hash[:])
	payload, err := json.Marshal(evidence)
	if err != nil {
		return "", err
	}
	if h.Artifacts == nil {
		return "", errors.New("evidence artifact store unavailable")
	}
	cid, err := h.Artifacts.Put(ctx, artifactstore.ContractScope(cmd.DID), payload)
	if err != nil {
		return "", err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO contract_evidence (evidence_id, contract_did, artifact_cid) VALUES ($1,$2,$3)`, *id, cmd.DID, cid); err != nil {
		return "", err
	}

	evt := contractevents.RecordEvidenceEvent{
		EvidenceID:      *id,
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		RecordedBy:      cmd.RecordedBy,
		OccurredAt:      now,
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return "", fmt.Errorf("could not create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return *id, nil
}
