package auth

import (
	"context"
	"testing"
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
