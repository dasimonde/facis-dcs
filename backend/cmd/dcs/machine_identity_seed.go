package main

import (
	"context"
	"fmt"

	"digital-contracting-service/internal/auth/machineidentity"
	"digital-contracting-service/internal/middleware"
)

// seedMachineIdentities writes the declaratively configured system clients into
// the registry, which is what every request resolves against.
//
// A deployment needs callers that exist before a human can log in to create
// them — the BDD harness authenticates as one to reach the API at all — so the
// declared set is reconciled on every start. Their Hydra clients come from the
// same deployment configuration, so no secret is issued here.
func seedMachineIdentities(ctx context.Context, repo machineidentity.Repo, seeded []middleware.SystemClient) error {
	for _, client := range seeded {
		roles, err := machineidentity.EncodeRoles(client.Roles)
		if err != nil {
			return fmt.Errorf("system client %q: %w", client.ClientID, err)
		}

		description := "Seeded from DCS_SYSTEM_CLIENTS."
		if err := repo.Upsert(ctx, machineidentity.Identity{
			Name:           client.ClientID,
			OAuthClientID:  client.ClientID,
			ParticipantDID: client.ParticipantDID,
			RolesJSON:      roles,
			Description:    &description,
			Enabled:        true,
			CreatedBy:      "deployment configuration",
		}); err != nil {
			return err
		}
	}
	return nil
}
