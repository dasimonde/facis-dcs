package command

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/semantichub"

	"github.com/jmoiron/sqlx"
)

// requireDeclaredShapeLibraries refuses a template whose declared shapes
// graphs this instance's Semantic Hub cannot resolve.
//
// A template crossing the Federated Catalogue keeps the sh:shapesGraph
// anchors it was authored under (ADR-8), and contract creation copies them
// into every contract derived from it — validation then loads exactly those
// graphs, so a graph the local hub does not hold is a hard failure at the
// consumer's first submit, far from the registration that caused it. Checked
// here, the registrant is told which library at which version is missing and
// where it was published, instead of the template landing in the repository
// unusable.
func requireDeclaredShapeLibraries(ctx context.Context, db *sqlx.DB, templateData *datatype.JSON) error {
	declared, err := validation.DeclaredShapeLibraries(templateData)
	if err != nil {
		return fmt.Errorf("could not read the shapes graphs the template declares: %w", err)
	}
	if len(declared) == 0 {
		return nil
	}

	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var missing []validation.DeclaredShapeLibrary
	for _, library := range declared {
		_, err := (semantichub.Repo{}).Get(ctx, tx, library.Name, "shapes", library.Version)
		if err == nil {
			continue
		}
		if errors.Is(err, semantichub.ErrSchemaNotFound) {
			missing = append(missing, library)
			continue
		}
		return fmt.Errorf("could not look up shapes %q in the Semantic Hub: %w", library.Name, err)
	}
	if len(missing) == 0 {
		return nil
	}
	return missingShapeLibrariesError(missing)
}

// missingShapeLibrariesError names every unresolvable graph, the version the
// template pins it at, and the hub URL it was published at — the three things
// an operator needs to publish the same library here before retrying.
func missingShapeLibrariesError(missing []validation.DeclaredShapeLibrary) error {
	details := make([]string, 0, len(missing))
	for _, library := range missing {
		version := fmt.Sprintf("version %d", library.Version)
		if library.Version == 0 {
			version = "no pinned version"
		}
		detail := fmt.Sprintf("%q (%s)", library.Name, version)
		if library.Anchor != "" {
			detail += fmt.Sprintf(", declared as %s", library.Anchor)
		}
		details = append(details, detail)
	}
	return fmt.Errorf(
		"this template is authored against shape libraries that are not in this instance's Semantic Hub: %s. "+
			"Publish each of them here under the same name and version, then register the template again",
		strings.Join(details, "; "))
}
