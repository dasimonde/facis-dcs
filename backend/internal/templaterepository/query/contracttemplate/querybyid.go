package contracttemplate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/datatype/eventtype"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type GetByIDQry struct {
	DID         string
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type GetByIDResult struct {
	DID          string
	Version      int
	State        contracttemplatestate.ContractTemplateState
	TemplateType contracttemplatetype.ContractTemplateType
	PartyType    string
	Name         *string
	Description  *string
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	TemplateData *datatype.JSON
	db.SearchableMetadata
	db.Structure
	// ReviewNotes is set for the template roles only.
	ReviewNotes []ReviewNote
}

type GetByIDHandler struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
}

func (h *GetByIDHandler) Handle(ctx context.Context, query GetByIDQry) (*GetByIDResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	data, err := h.CTRepo.ReadDataByID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not get contract template data: %w", err)
	}

	// DCS-FR-TR-19: a template user without a template role reads only
	// templates that were registered. A deprecated one stays readable for
	// the contracts made from it.
	var notes []ReviewNote
	if query.UserRoles.HasRoles(templateRoles...) {
		// DCS-IR-TR-04, DCS-IR-TR-05: the template roles see the review
		// comments and the rejection reason.
		notes, err = h.reviewNotes(ctx, tx, query.DID)
		if err != nil {
			return nil, err
		}
	} else if !slices.Contains(registeredStates, data.State) {
		return nil, fmt.Errorf("%w: did=%s", db.ErrContractTemplateNotFound, query.DID)
	}

	evt := templateevents.RetrieveByIDEvent{
		DID:         query.DID,
		Version:     data.Version,
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	state, err := contracttemplatestate.NewContractTemplateState(data.State)
	if err != nil {
		return nil, fmt.Errorf("could not create contract template state: %w", err)
	}

	templateType, err := contracttemplatetype.NewContractTemplateType(data.TemplateType)
	if err != nil {
		return nil, fmt.Errorf("could not create contract template type: %w", err)
	}

	return &GetByIDResult{
		DID:          query.DID,
		Version:      data.Version,
		State:        state,
		TemplateType: templateType,
		PartyType:    data.PartyType,
		Name:         data.Name,
		Description:  data.Description,
		CreatedBy:    data.CreatedBy,
		CreatedAt:    data.CreatedAt,
		UpdatedAt:    data.UpdatedAt,
		TemplateData: data.TemplateData,

		SearchableMetadata: data.SearchableMetadata,
		Structure:          data.Structure,
		ReviewNotes:        notes,
	}, nil
}

var templateRoles = []userrole.UserRole{userrole.TemplateCreator, userrole.TemplateReviewer, userrole.TemplateApprover, userrole.TemplateManager}

var registeredStates = []string{
	contracttemplatestate.Registered.String(),
	contracttemplatestate.Published.String(),
	contracttemplatestate.Deprecated.String(),
}

// Kinds of review notes.
const (
	NoteReturnedToDraft     = "RETURNED_TO_DRAFT"
	NoteForwardedToApproval = "FORWARDED_TO_APPROVAL"
	NoteReturnedToReview    = "RETURNED_TO_REVIEW"
	NoteRejected            = "REJECTED"
	NoteApproved            = "APPROVED"
)

// ReviewNote is a comment of a review or approval decision, or a rejection reason.
type ReviewNote struct {
	Kind      string
	Text      []string
	By        string
	CreatedAt time.Time
}

// reviewNotes reads the review and approval decisions with text from the
// template's events, oldest first.
func (h *GetByIDHandler) reviewNotes(ctx context.Context, tx *sqlx.Tx, did string) ([]ReviewNote, error) {
	events, err := h.CTRepo.ReadEvents(ctx, tx, did, eventtype.Submit.String(), eventtype.Reject.String(), eventtype.Approve.String())
	if err != nil {
		return nil, err
	}
	notes := []ReviewNote{}
	for _, evt := range events {
		var data struct {
			PreviousState string   `json:"previous_state"`
			NewState      string   `json:"new_state"`
			Comments      []string `json:"comments"`
			Reason        string   `json:"reason"`
			DecisionNotes []string `json:"decision_notes"`
			HolderDID     string   `json:"holder_did"`
		}
		if err := json.Unmarshal(evt.EventData, &data); err != nil {
			return nil, fmt.Errorf("decode %s event of %s: %w", evt.EventType, did, err)
		}
		note := ReviewNote{By: data.HolderDID, CreatedAt: evt.CreatedAt}
		switch evt.EventType {
		case eventtype.Reject.String():
			note.Kind, note.Text = NoteRejected, []string{data.Reason}
		case eventtype.Approve.String():
			note.Kind, note.Text = NoteApproved, data.DecisionNotes
		case eventtype.Submit.String():
			note.Text = data.Comments
			switch {
			case data.NewState == contracttemplatestate.Reviewed.String():
				note.Kind = NoteForwardedToApproval
			case data.PreviousState == contracttemplatestate.Submitted.String():
				note.Kind = NoteReturnedToDraft
			case data.PreviousState == contracttemplatestate.Reviewed.String():
				note.Kind = NoteReturnedToReview
			}
		}
		if note.Kind != "" && hasText(note.Text) {
			notes = append(notes, note)
		}
	}
	return notes, nil
}

func hasText(text []string) bool {
	for _, line := range text {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}
	return false
}
