// Package templatestructure holds the hierarchy level of a contract template,
// the templates it depends on, and the structural rules between the contracts
// made from them (DCS-FR-TR-02, DCS-FR-TR-23, DCS-FR-TR-26, DCS-FR-PACM-06).
//
// A dependency points up the hierarchy: a sub-agreement template requires a
// frame agreement template, and a contract made from it needs a parent
// contract made from that template.
package templatestructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// ErrInvalid is a client-input error (map to 4xx): a template or a contract
// hierarchy breaks a structural rule.
var ErrInvalid = errors.New("invalid template structure")

type Level string

const (
	FrameAgreement Level = "FRAME_AGREEMENT"
	Agreement      Level = "AGREEMENT"
	SubAgreement   Level = "SUB_AGREEMENT"
	Addendum       Level = "ADDENDUM"
)

// rank orders the levels from the top of the hierarchy.
func rank(level Level) int {
	switch level {
	case FrameAgreement:
		return 0
	case Addendum:
		return 2
	default:
		return 1
	}
}

// Template is the structure of one template version. Family is the template
// lineage (base_template); Requires holds the families of the templates it
// depends on.
type Template struct {
	DID      string         `db:"did"`
	Family   string         `db:"family"`
	Level    Level          `db:"template_level"`
	Requires pq.StringArray `db:"requires"`
}

// CheckDependency refuses a dependency that does not point up the hierarchy.
func CheckDependency(level, dependency Level) error {
	if rank(dependency) >= rank(level) {
		return fmt.Errorf("%w: a %s template cannot depend on a %s template", ErrInvalid, level, dependency)
	}
	return nil
}

// CheckParent checks the parent of a contract made from child. parent is nil
// when the contract has no parent contract.
func CheckParent(child Template, parent *Template) error {
	if parent == nil {
		if child.Level == SubAgreement || child.Level == Addendum || len(child.Requires) > 0 {
			return fmt.Errorf("%w: a contract from %s template %s needs a parent contract", ErrInvalid, child.Level, child.DID)
		}
		return nil
	}
	switch {
	case child.Level == FrameAgreement:
		return fmt.Errorf("%w: a frame agreement has no parent contract", ErrInvalid)
	case parent.Level == Addendum:
		return fmt.Errorf("%w: an addendum has no child contracts", ErrInvalid)
	case child.Level == SubAgreement && parent.Level != FrameAgreement:
		return fmt.Errorf("%w: the parent of a sub-agreement must be a frame agreement, not %s", ErrInvalid, parent.Level)
	case len(child.Requires) > 0 && !slices.Contains(child.Requires, parent.Family):
		return fmt.Errorf("%w: template %s requires a parent contract from template %v, the parent is from %s",
			ErrInvalid, child.DID, []string(child.Requires), parent.Family)
	}
	return nil
}

const templateColumns = `
	t.did, COALESCE(t.base_template, t.did) AS family, t.template_level,
	ARRAY(SELECT DISTINCT COALESCE(d.base_template, d.did) FROM contract_templates d
	      WHERE d.did = ANY(t.required_templates)) AS requires`

// LoadForContract reads the structure of the template the contract was made
// from. It returns nil when the contract or its template is not known on this
// instance.
func LoadForContract(ctx context.Context, q sqlx.QueryerContext, contractDID string) (*Template, error) {
	var tpl Template
	err := sqlx.GetContext(ctx, q, &tpl, `SELECT `+templateColumns+`
		FROM contracts c JOIN contract_templates t ON t.did = c.template_did
		WHERE c.did = $1`, contractDID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read template structure of contract %s: %w", contractDID, err)
	}
	return &tpl, nil
}

// CheckContractParent checks the parent of a contract when both contracts
// and their templates are known on this instance.
func CheckContractParent(ctx context.Context, q sqlx.QueryerContext, contractDID, parentDID string) error {
	child, err := LoadForContract(ctx, q, contractDID)
	if err != nil || child == nil {
		return err
	}
	parent, err := LoadForContract(ctx, q, parentDID)
	if err != nil || parent == nil {
		return err
	}
	return CheckParent(*child, parent)
}

// usableStates are the template states a dependency can be used in.
var usableStates = pq.StringArray{"APPROVED", "REGISTERED", "PUBLISHED"}

// CheckTemplate validates the dependencies of a template: each required
// template exists, is a contract template of a higher level, is not the
// template itself, and has a version that is approved or registered.
func CheckTemplate(ctx context.Context, q sqlx.QueryerContext, did string) error {
	var tpl struct {
		Family   string         `db:"family"`
		Level    Level          `db:"template_level"`
		Required pq.StringArray `db:"required_templates"`
	}
	if err := sqlx.GetContext(ctx, q, &tpl, `
		SELECT COALESCE(base_template, did) AS family, template_level, required_templates
		FROM contract_templates WHERE did = $1`, did); err != nil {
		return fmt.Errorf("read template structure of %s: %w", did, err)
	}
	for _, required := range tpl.Required {
		var dep struct {
			Family       string `db:"family"`
			Level        Level  `db:"template_level"`
			TemplateType string `db:"template_type"`
			Usable       bool   `db:"usable"`
		}
		err := sqlx.GetContext(ctx, q, &dep, `
			SELECT COALESCE(t.base_template, t.did) AS family, t.template_level, t.template_type,
			       EXISTS (SELECT 1 FROM contract_templates v
			               WHERE COALESCE(v.base_template, v.did) = COALESCE(t.base_template, t.did)
			                 AND v.state::text = ANY($2)) AS usable
			FROM contract_templates t WHERE t.did = $1`, required, usableStates)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return fmt.Errorf("%w: required template %s does not exist", ErrInvalid, required)
		case err != nil:
			return fmt.Errorf("read required template %s: %w", required, err)
		case dep.Family == tpl.Family:
			return fmt.Errorf("%w: template %s cannot require itself", ErrInvalid, did)
		case dep.TemplateType != "CONTRACT_TEMPLATE":
			return fmt.Errorf("%w: required template %s is a %s, not a contract template", ErrInvalid, required, dep.TemplateType)
		case !dep.Usable:
			return fmt.Errorf("%w: required template %s has no approved or registered version", ErrInvalid, required)
		}
		if err := CheckDependency(tpl.Level, dep.Level); err != nil {
			return err
		}
	}
	return nil
}
