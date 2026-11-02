package auth

import (
	"context"
	"fmt"
	"testing"

	goa "goa.design/goa/v3/pkg"

	"digital-contracting-service/internal/auth/db"
)

type accessMetadataRequest struct {
	Scope         string
	Did           *string
	Justification string
}

func TestAccessMetadataMiddlewareMakesDecodedFieldsAvailableToAuthentication(t *testing.T) {
	did := "did:web:contract"
	next := AccessMetadataMiddleware(func(ctx context.Context, request any) (any, error) {
		metadata := accessMetadataFromContext(ctx)
		if metadata.Scope != "archive" || metadata.DID == nil || *metadata.DID != did || metadata.Justification != "investigation" {
			t.Fatalf("unexpected metadata: %+v", metadata)
		}
		return nil, nil
	})
	_, err := next(context.Background(), &accessMetadataRequest{Scope: "archive", Did: &did, Justification: "investigation"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAccessFinalizerRecordsFinalDecisionOnce(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		outcome string
	}{
		{"success", nil, db.AccessOutcomeSuccess},
		{"forbidden", goa.PermanentError("forbidden", "role"), db.AccessOutcomeDenied},
		{"unauthorized", fmt.Errorf("wrapped: %w", goa.PermanentError("unauthorized", "scope")), db.AccessOutcomeDenied},
		{"business error", goa.PermanentError("internal_error", "executor"), db.AccessOutcomeFailed},
		{"bad request", goa.PermanentError("bad_request", "state"), db.AccessOutcomeFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			endpoint := AccessMetadataMiddleware(func(ctx context.Context, _ any) (any, error) {
				ctx.Value(accessFinalizerKey{}).(*accessFinalizer).record = func(err error) error {
					calls++
					if outcome := accessOutcome(err); outcome != tc.outcome {
						t.Fatalf("outcome = %s", outcome)
					}
					return nil
				}
				if calls != 0 {
					t.Fatal("attempt finalized before endpoint returned")
				}
				return nil, tc.err
			})
			_, _ = endpoint(context.Background(), nil)
			if calls != 1 {
				t.Fatalf("recorded %d attempts", calls)
			}
		})
	}
}

func TestAccessMetadataStringDIDAndResourceAlias(t *testing.T) {
	for _, request := range []any{
		struct{ Did string }{Did: "did:web:resource"},
		&struct{ ResourceID string }{ResourceID: "did:web:resource"},
	} {
		endpoint := AccessMetadataMiddleware(func(ctx context.Context, _ any) (any, error) {
			metadata := accessMetadataFromContext(ctx)
			if metadata.DID == nil || *metadata.DID != "did:web:resource" {
				t.Fatalf("metadata = %+v", metadata)
			}
			return nil, nil
		})
		_, _ = endpoint(context.Background(), request)
	}
}

// CSA-PACM-17 (DCS-FR-PACM-04): an audit read, which states a justification,
// is not answered when its access record cannot be written. Another request
// is answered.
func TestAnAuditReadIsNotServedWithoutItsAccessRecord(t *testing.T) {
	endpoint := AccessMetadataMiddleware(func(ctx context.Context, _ any) (any, error) {
		ctx.Value(accessFinalizerKey{}).(*accessFinalizer).record = func(error) error {
			return fmt.Errorf("database unavailable")
		}
		return "audit log", nil
	})

	result, err := endpoint(context.Background(), &accessMetadataRequest{Justification: "investigation"})
	if err == nil || result != nil {
		t.Fatalf("the audit read was served without its access record: %v, %v", result, err)
	}
	if result, err := endpoint(context.Background(), &accessMetadataRequest{}); err != nil || result != "audit log" {
		t.Fatalf("a request without a justification was refused: %v, %v", result, err)
	}
}
